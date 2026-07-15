import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MockWebSocket } from '../shared/test/mocks/websocket.js';

const connectMocks = vi.hoisted(() => ({
  establishGameSession: vi.fn(async (): Promise<import('../shared/network/session.js').SessionResult> => ({ ok: true })),
  validateRoomCode: vi.fn(async (): Promise<import('./room_validate.js').RoomValidateResult> => ({ ok: true })),
  resolveLobbyCode: vi.fn(async (): Promise<string | null> => 'ROOM2'),
  getLobbyCodeFromUrl: vi.fn((): string | null => 'URL22'),
  enqueueBinaryMessage: vi.fn(),
  onLobbyCodeReady: vi.fn(),
  getEntryStep: vi.fn((): import('./entry_flow.js').EntryStep => 'connecting'),
  ws: null as MockWebSocket | { readyState: number; close: ReturnType<typeof vi.fn>; onclose: null } | null,
  wsEverOpened: false,
  roomPreChecked: false,
}));

vi.mock('../shared/network/session.js', () => ({
  establishGameSession: connectMocks.establishGameSession,
  sessionErrorMessage: () => 'auth failed',
}));

vi.mock('./room_validate.js', () => ({
  ROOM_CODE_RE: /^[A-Z2-9]{5}$/,
  getLobbyCodeFromUrl: connectMocks.getLobbyCodeFromUrl,
  validateRoomCode: connectMocks.validateRoomCode,
  roomErrorMessage: () => 'bad room',
}));

vi.mock('./lobby_match.js', () => ({
  resolveLobbyCode: connectMocks.resolveLobbyCode,
}));

vi.mock('./entry_flow.js', () => ({
  onLobbyCodeReady: connectMocks.onLobbyCodeReady,
  getEntryStep: connectMocks.getEntryStep,
  onWebSocketOpen: vi.fn(),
  onWebSocketClosed: vi.fn(),
  routeConnectionError: vi.fn(),
  clearWaitingInlineError: vi.fn(),
}));

vi.mock('./ws_connection.js', () => ({
  setWs: vi.fn((socket: typeof connectMocks.ws) => { connectMocks.ws = socket; }),
  getWs: vi.fn(() => connectMocks.ws as WebSocket | null),
  getWsEverOpened: vi.fn(() => connectMocks.wsEverOpened),
  setWsEverOpened: vi.fn((value: boolean) => { connectMocks.wsEverOpened = value; }),
  resetReconnectAttempts: vi.fn(),
  setReconnectTimer: vi.fn(),
  startHeartbeat: vi.fn(),
  stopHeartbeat: vi.fn(),
  sendOrQueue: vi.fn(),
  flushPendingQueue: vi.fn(),
  hideReconnectBanner: vi.fn(),
  scheduleReconnect: vi.fn(),
  showConnectionError: vi.fn(),
  wasRoomPreChecked: vi.fn(() => connectMocks.roomPreChecked),
  setRoomPreChecked: vi.fn((value: boolean) => { connectMocks.roomPreChecked = value; }),
}));

vi.mock('./ws_message_queue.js', () => ({
  enqueueBinaryMessage: connectMocks.enqueueBinaryMessage,
}));

vi.stubGlobal('WebSocket', MockWebSocket);
vi.stubGlobal('localStorage', {
  getItem: vi.fn(() => null),
  setItem: vi.fn(),
});

import { connectWebSocket } from './ws_connect.js';
import { dispatch } from './store.js';

describe('connectWebSocket', () => {
  beforeEach(() => {
    dispatch({ type: 'RESET_ALL' });
    vi.clearAllMocks();
    MockWebSocket.lastInstance = null;
    connectMocks.ws = null;
    connectMocks.wsEverOpened = false;
    connectMocks.roomPreChecked = false;
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
    expect(connectMocks.enqueueBinaryMessage).toHaveBeenCalledWith(buf);
  });

  it('shows error when session establishment fails', async () => {
    const { showConnectionError } = await import('./ws_connection.js');
    connectMocks.establishGameSession.mockResolvedValueOnce({ ok: false, reason: 'network' } as import('../shared/network/session.js').SessionResult);
    await connectWebSocket();
    expect(showConnectionError).toHaveBeenCalled();
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
    const { showConnectionError } = await import('./ws_connection.js');
    connectMocks.validateRoomCode.mockResolvedValueOnce({ ok: false, reason: 'ended' });
    await connectWebSocket();
    expect(showConnectionError).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ title: '房间已结束' }),
    );
  });

  it('shows error when lobby match returns null', async () => {
    const { showConnectionError } = await import('./ws_connection.js');
    connectMocks.getLobbyCodeFromUrl.mockReturnValueOnce(null);
    connectMocks.resolveLobbyCode.mockResolvedValueOnce(null);
    await connectWebSocket();
    expect(showConnectionError).toHaveBeenCalledWith(
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

  it('skips reconnect when socket already open for same lobby', async () => {
    await connectWebSocket();
    expect(MockWebSocket.lastInstance).not.toBeNull();
    const first = MockWebSocket.lastInstance;
    await connectWebSocket();
    expect(MockWebSocket.lastInstance).toBe(first);
  });

  it('uses fresh match sessionStorage without re-validating room', async () => {
    sessionStorage.setItem('uppy-fresh-match', 'URL22');
    await connectWebSocket();
    expect(connectMocks.validateRoomCode).not.toHaveBeenCalled();
    expect(connectMocks.roomPreChecked).toBe(true);
    sessionStorage.clear();
    connectMocks.roomPreChecked = false;
  });

  it('shows connection error when socket closes before open', async () => {
    const { showConnectionError, setWsEverOpened } = await import('./ws_connection.js');
    await connectWebSocket();
    setWsEverOpened(false);
    MockWebSocket.lastInstance?.onclose?.();
    expect(showConnectionError).toHaveBeenCalled();
  });

  it('ignores non-arraybuffer websocket messages', async () => {
    await connectWebSocket();
    MockWebSocket.lastInstance?.onmessage?.({ data: 'text' } as MessageEvent);
    expect(connectMocks.enqueueBinaryMessage).not.toHaveBeenCalled();
  });

  it('logs websocket errors without throwing', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    await connectWebSocket();
    expect(typeof MockWebSocket.lastInstance?.onerror).toBe('function');
    MockWebSocket.lastInstance!.onerror!(new Event('error'));
    expect(errSpy).toHaveBeenCalledWith('WebSocket error');
    errSpy.mockRestore();
  });

  it('shows not_found room error title', async () => {
    const { showConnectionError } = await import('./ws_connection.js');
    connectMocks.validateRoomCode.mockResolvedValueOnce({ ok: false, reason: 'not_found' });
    await connectWebSocket();
    expect(showConnectionError).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ title: '无法进入房间' }),
    );
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

  it('fires onopen handlers and restart vote after ended phase', async () => {
    const { sendOrQueue } = await import('./ws_connection.js');
    const { state } = await import('./state_types.js');
    state.phase = 'ended';
    state.restartClicked = true;
    await connectWebSocket();
    MockWebSocket.lastInstance?.onopen?.();
    expect(sendOrQueue).toHaveBeenCalled();
    state.phase = 'waiting';
    state.restartClicked = false;
  });

  it('schedules reconnect when socket closes after it opened', async () => {
    const { scheduleReconnect, setWsEverOpened } = await import('./ws_connection.js');
    await connectWebSocket();
    setWsEverOpened(true);
    MockWebSocket.lastInstance?.onclose?.();
    expect(scheduleReconnect).toHaveBeenCalled();
  });

  it('skips second connect when socket already open after room resolve', async () => {
    connectMocks.getEntryStep.mockReturnValue('waiting');
    connectMocks.getLobbyCodeFromUrl.mockReturnValue('ROOM2');
    await connectWebSocket();
    const first = MockWebSocket.lastInstance;
    connectMocks.getLobbyCodeFromUrl.mockReturnValue('ROOM2');
    await connectWebSocket();
    expect(MockWebSocket.lastInstance).toBe(first);
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

  it('shows room-prechecked error when socket closes before open', async () => {
    const { showConnectionError, setWsEverOpened, setRoomPreChecked } = await import('./ws_connection.js');
    setRoomPreChecked(true);
    await connectWebSocket();
    setWsEverOpened(false);
    MockWebSocket.lastInstance?.onclose?.();
    expect(showConnectionError).toHaveBeenCalledWith(
      '无法连接房间，请稍后重试',
      expect.objectContaining({ showActions: true }),
    );
    setRoomPreChecked(false);
  });

  it('skips reconnect after match when socket is already open', async () => {
    connectMocks.getLobbyCodeFromUrl.mockReturnValue(null);
    connectMocks.resolveLobbyCode.mockResolvedValue('MATCH2');
    connectMocks.getEntryStep.mockReturnValue('waiting');
    await connectWebSocket();
    const first = MockWebSocket.lastInstance;
    await connectWebSocket();
    expect(MockWebSocket.lastInstance).toBe(first);
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
