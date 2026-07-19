import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MockWebSocket } from '../shared/test/mocks/websocket.js';

const connectMocks = vi.hoisted(() => ({
  establishGameSession: vi.fn(async (): Promise<import('../shared/network/session.js').SessionResult> => ({ ok: true })),
  validateRoomCode: vi.fn(async (): Promise<import('./lobby_match.js').RoomValidateResult> => ({ ok: true })),
  resolveLobbyCode: vi.fn(async (): Promise<string | null> => 'ROOM2'),
  getLobbyCodeFromUrl: vi.fn((): string | null => 'URL22'),
  onLobbyCodeReady: vi.fn(),
  getEntryStep: vi.fn((): import('./entry_flow.js').EntryStep => 'connecting'),
}));

vi.mock('../shared/network/session.js', () => ({
  establishGameSession: connectMocks.establishGameSession,
  sessionErrorMessage: () => 'auth failed',
}));

vi.mock('./lobby_match.js', () => ({
  ROOM_CODE_RE: /^[A-Z2-9]{5}$/,
  getLobbyCodeFromUrl: connectMocks.getLobbyCodeFromUrl,
  validateRoomCode: connectMocks.validateRoomCode,
  roomErrorMessage: () => 'bad room',
  resolveLobbyCode: connectMocks.resolveLobbyCode,
}));

vi.mock('./entry_flow.js', () => ({
  onLobbyCodeReady: connectMocks.onLobbyCodeReady,
  getEntryStep: connectMocks.getEntryStep,
  onWebSocketOpen: vi.fn(),
  onWebSocketClosed: vi.fn(),
  clearWaitingInlineError: vi.fn(),
  showLoadingOverlay: vi.fn(),
}));

vi.mock('./ui_common.js', async () => {
  const { createUiCommonMock } = await import('./ws_connection_test_setup.js');
  return createUiCommonMock();
});

vi.mock('./state_interp.js', () => ({
  resetInterpolation: vi.fn(),
}));

vi.mock('./seen_seqs.js', () => ({
  clearSeenSeqs: vi.fn(),
}));

vi.mock('./ws_handlers.js', () => ({
  handleBinaryMessage: vi.fn(),
}));

vi.stubGlobal('WebSocket', MockWebSocket);
vi.stubGlobal('localStorage', {
  getItem: vi.fn(() => null),
  setItem: vi.fn(),
});

import {
  connectWebSocket,
  setWs,
  setWsEverOpened,
  setRoomPreChecked,
  wasRoomPreChecked,
  resetReconnectAttempts,
  clearReconnectTimer,
  clearOutboundQueue,
  drainPendingMessages,
} from './ws_connection.js';
import { dispatch } from './state.js';
import { showConnectionError as showConnectionErrorUI, showReconnectBanner } from './ui_common.js';
import { handleBinaryMessage } from './ws_handlers.js';

describe('connectWebSocket', () => {
  beforeEach(() => {
    dispatch({ type: 'RESET_ALL' });
    vi.clearAllMocks();
    MockWebSocket.lastInstance = null;
    setWs(null);
    setWsEverOpened(false);
    setRoomPreChecked(false);
    resetReconnectAttempts();
    clearReconnectTimer();
    clearOutboundQueue();
    connectMocks.getLobbyCodeFromUrl.mockReturnValue('URL22');
    connectMocks.getEntryStep.mockReturnValue('connecting');
    connectMocks.establishGameSession.mockResolvedValue({ ok: true });
    connectMocks.validateRoomCode.mockResolvedValue({ ok: true });
    connectMocks.resolveLobbyCode.mockResolvedValue('ROOM2');
  });

  it('routes inbound frames through the message queue', async () => {
    await connectWebSocket();
    const buf = new ArrayBuffer(4);
    MockWebSocket.lastInstance?.onmessage?.({ data: buf } as MessageEvent);
    drainPendingMessages(1);
    expect(handleBinaryMessage).toHaveBeenCalledWith(buf);
  });

  it('shows error when session establishment fails', async () => {
    connectMocks.establishGameSession.mockResolvedValueOnce({ ok: false, reason: 'network' } as import('../shared/network/session.js').SessionResult);
    await connectWebSocket();
    expect(showConnectionErrorUI).toHaveBeenCalled();
  });

  it('calls onLobbyCodeReady before session when URL has code', async () => {
    const order: string[] = [];
    connectMocks.onLobbyCodeReady.mockImplementation((code: string) => {
      order.push(`ready:${code}`);
    });
    connectMocks.establishGameSession.mockImplementation(async () => {
      order.push('session');
      return { ok: true as const };
    });
    await connectWebSocket();
    expect(order[0]).toBe('ready:URL22');
    expect(order[1]).toBe('session');
  });

  it('does not call onLobbyCodeReady again when already past connecting', async () => {
    connectMocks.getEntryStep.mockReturnValue('waiting');
    await connectWebSocket();
    expect(connectMocks.onLobbyCodeReady).not.toHaveBeenCalled();
  });

  it('shows error when room validation fails', async () => {
    connectMocks.validateRoomCode.mockResolvedValueOnce({ ok: false, reason: 'ended' });
    await connectWebSocket();
    expect(showConnectionErrorUI).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ title: '房间已结束' }),
    );
  });

  it('shows error when lobby match returns null', async () => {
    connectMocks.getLobbyCodeFromUrl.mockReturnValueOnce(null);
    connectMocks.resolveLobbyCode.mockResolvedValueOnce(null);
    await connectWebSocket();
    expect(showConnectionErrorUI).toHaveBeenCalledWith(
      '无法连接到游戏服务器，请稍后重试',
      expect.objectContaining({ showActions: true }),
    );
  });

  it('publishes matched lobby code before opening socket', async () => {
    connectMocks.getLobbyCodeFromUrl.mockReturnValueOnce(null);
    connectMocks.resolveLobbyCode.mockResolvedValueOnce('MATCH2');
    await connectWebSocket();
    expect(connectMocks.onLobbyCodeReady).toHaveBeenCalledWith('MATCH2');
  });

  it('uses fresh match sessionStorage without re-validating room', async () => {
    sessionStorage.setItem('uppy-fresh-match', 'URL22');
    await connectWebSocket();
    expect(connectMocks.validateRoomCode).not.toHaveBeenCalled();
    expect(wasRoomPreChecked()).toBe(true);
    sessionStorage.clear();
    setRoomPreChecked(false);
  });

  it('logs websocket errors without throwing', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    await connectWebSocket();
    expect(typeof MockWebSocket.lastInstance?.onerror).toBe('function');
    MockWebSocket.lastInstance!.onerror!(new Event('error'));
    expect(errSpy).toHaveBeenCalledWith('WebSocket error');
    errSpy.mockRestore();
  });

  it('ignores connect while another connect is in flight', async () => {
    let resolveSession: (value: import('../shared/network/session.js').SessionResult) => void = () => {};
    connectMocks.establishGameSession.mockImplementation(
      () => new Promise((resolve) => { resolveSession = resolve; }),
    );
    const first = connectWebSocket();
    await connectWebSocket();
    resolveSession({ ok: true });
    await first;
    expect(MockWebSocket.lastInstance).not.toBeNull();
  });

  it('schedules reconnect when socket closes after it opened', async () => {
    await connectWebSocket();
    setWsEverOpened(true);
    MockWebSocket.lastInstance?.onclose?.();
    expect(showReconnectBanner).toHaveBeenCalled();
  });

  it('closes an existing socket before opening a new lobby connection', async () => {
    await connectWebSocket();
    const first = MockWebSocket.lastInstance!;
    connectMocks.getLobbyCodeFromUrl.mockReturnValue('OTHER');
    connectMocks.getEntryStep.mockReturnValue('waiting');
    await connectWebSocket();
    expect(first.close).toHaveBeenCalled();
    expect(MockWebSocket.lastInstance).not.toBe(first);
  });

  it('uses wss protocol when page is served over https', async () => {
    vi.stubGlobal('location', {
      ...window.location,
      protocol: 'https:',
      host: 'game.test',
    });
    await connectWebSocket();
    expect(MockWebSocket.lastInstance?.url).toBe('wss://game.test/api/v1/lobby/URL22/ws');
    vi.unstubAllGlobals();
  });
});