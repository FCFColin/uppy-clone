import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockWebSocket } from '../shared/test/mocks/websocket.js';

// Shared mock functions reconfigured per describe block.
const sharedMocks = vi.hoisted(() => ({
  establishGameSession: vi.fn(),
  validateRoomCode: vi.fn(),
  resolveLobbyCode: vi.fn(),
  getLobbyCodeFromUrl: vi.fn(),
  onLobbyCodeReady: vi.fn(),
  onWebSocketOpen: vi.fn(),
  onWebSocketClosed: vi.fn(),
  getEntryStep: vi.fn(),
  clearWaitingInlineError: vi.fn(),
  showLoadingOverlay: vi.fn(),
}));

// Mutable heartbeat config so different describe blocks can use different
// timeout/interval values without separate vi.mock calls. The mock below
// exposes these as getters so ws_connection.ts reads the current value at
// runtime (inside interval/timeout callbacks), not at import time.
const heartbeatConfig = vi.hoisted(() => ({
  intervalMs: 1000,
  timeoutMs: 1500,
}));

vi.mock('./constants.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./constants.js')>();
  return {
    ...actual,
    get HEARTBEAT_INTERVAL_MS() {
      return heartbeatConfig.intervalMs;
    },
    get HEARTBEAT_TIMEOUT_MS() {
      return heartbeatConfig.timeoutMs;
    },
  };
});

vi.mock('../shared/network/network.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../shared/network/network.js')>();
  return {
    ...actual,
    establishGameSession: sharedMocks.establishGameSession,
    sessionErrorMessage: () => 'auth failed',
  };
});

vi.mock('./lobby_match.js', () => ({
  ROOM_CODE_RE: /^[A-Z2-9]{5}$/,
  getLobbyCodeFromUrl: sharedMocks.getLobbyCodeFromUrl,
  validateRoomCode: sharedMocks.validateRoomCode,
  roomErrorMessage: () => 'bad room',
  resolveLobbyCode: sharedMocks.resolveLobbyCode,
}));

vi.mock('./entry_flow.js', () => ({
  onLobbyCodeReady: sharedMocks.onLobbyCodeReady,
  onWebSocketOpen: sharedMocks.onWebSocketOpen,
  onWebSocketClosed: sharedMocks.onWebSocketClosed,
  getEntryStep: sharedMocks.getEntryStep,
  clearWaitingInlineError: sharedMocks.clearWaitingInlineError,
  showLoadingOverlay: sharedMocks.showLoadingOverlay,
}));

vi.mock('./ui_common.js', async () => {
  const { createUiCommonMock } = await import('./ws_handlers_test_setup.js');
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

import { MAX_PENDING_QUEUE, HEARTBEAT_INTERVAL_MS, HEARTBEAT_TIMEOUT_MS, MAX_RECONNECT_ATTEMPTS } from './constants.js';
import {
  setWs,
  stopHeartbeat,
  startHeartbeat,
  sendOrQueue,
  flushPendingQueue,
  resetReconnectAttempts,
  scheduleReconnect,
  setRoomPreChecked,
  wasRoomPreChecked,
  clearReconnectTimer,
  setWsEverOpened,
  getOutboundQueueLength,
  clearOutboundQueue,
  connectWebSocket,
  drainPendingMessages,
} from './ws_connection.js';
import { dispatch } from './state.js';
import { showConnectionError as showConnectionErrorUI, showReconnectBanner } from './ui_common.js';
import { handleBinaryMessage } from './ws_handlers.js';

describe('ws_connection', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    clearOutboundQueue();
    stopHeartbeat();
    setWs(null);
    resetReconnectAttempts();
    // Use timeout < interval so the timeout fires before the next interval
    // tick clears it (required by "heartbeat timeout closes the socket" test).
    heartbeatConfig.intervalMs = 1000;
    heartbeatConfig.timeoutMs = 500;
    sharedMocks.establishGameSession.mockRejectedValue(new Error('test-network'));
    sharedMocks.getLobbyCodeFromUrl.mockReturnValue(null);
    sharedMocks.resolveLobbyCode.mockRejectedValue(new Error('test-match'));
    sharedMocks.getEntryStep.mockReturnValue('waiting');
  });

  afterEach(() => {
    vi.useRealTimers();
    stopHeartbeat();
    // Restore defaults for other describe blocks (heartbeat reset test
    // needs timeout > interval so the second tick clears the first timeout).
    heartbeatConfig.timeoutMs = 1500;
  });

  it('sendOrQueue sends when socket open, queues when closed', () => {
    // When socket open: sends immediately
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    sendOrQueue(new ArrayBuffer(1));
    expect((socket as unknown as MockWebSocket).sent.length).toBe(1);

    // When socket closed: queues
    setWs(null);
    sendOrQueue(new ArrayBuffer(1));
    expect(getOutboundQueueLength()).toBe(1);
  });

  it('sendOrQueue drops oldest when queue full', () => {
    for (let i = 0; i < MAX_PENDING_QUEUE + 2; i++) {
      sendOrQueue(new ArrayBuffer(1));
    }
    expect(getOutboundQueueLength()).toBe(MAX_PENDING_QUEUE);
  });

  it('flushPendingQueue drains all queued messages on open socket, no-ops on closed', () => {
    // No-op when socket closed
    sendOrQueue(new ArrayBuffer(1));
    flushPendingQueue();
    expect(getOutboundQueueLength()).toBe(1);

    // Drains 1 message on open socket
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(null);
    sendOrQueue(new ArrayBuffer(1));
    sendOrQueue(new ArrayBuffer(1));
    sendOrQueue(new ArrayBuffer(1));
    setWs(socket);
    flushPendingQueue();
    expect(getOutboundQueueLength()).toBe(0);
    expect((socket as unknown as MockWebSocket).sent.length).toBe(4);
  });

  it('heartbeat timeout closes the socket when pong is missing', async () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    const closeSpy = vi.spyOn(socket, 'close');
    setWs(socket);
    startHeartbeat();
    await vi.advanceTimersByTimeAsync(HEARTBEAT_INTERVAL_MS + HEARTBEAT_TIMEOUT_MS + 1);
    expect(closeSpy).toHaveBeenCalled();
  });

  it('scheduleReconnect shows banner before max attempts, then connection error after max', async () => {
    // L-3: scheduleReconnect 触发 connectWebSocket，establishGameSession mock 永远 reject，
    // reconnect 失败会 console.error('reconnect failed:', e)，属预期行为但日志噪声大
    vi.spyOn(console, 'error').mockImplementation(() => {});
    scheduleReconnect();
    expect(showReconnectBanner).toHaveBeenCalledWith(1);
    for (let i = 1; i < MAX_RECONNECT_ATTEMPTS; i++) {
      scheduleReconnect();
      vi.runAllTimers();
    }
    scheduleReconnect();
    await vi.waitFor(() => {
      expect(showConnectionErrorUI).toHaveBeenCalled();
    });
  });

});

describe('ws_connection heartbeat reset', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // timeout > interval: the second interval tick must clear the first
    // timeout before it fires (otherwise close would be called).
    heartbeatConfig.intervalMs = 1000;
    heartbeatConfig.timeoutMs = 1500;
    stopHeartbeat();
    setWs(null);
  });

  afterEach(() => {
    vi.useRealTimers();
    stopHeartbeat();
  });

  it('clears the prior timeout when a subsequent ping is sent', async () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    const closeSpy = vi.spyOn(socket, 'close');
    setWs(socket);
    startHeartbeat();
    await vi.advanceTimersByTimeAsync(HEARTBEAT_INTERVAL_MS * 2);
    expect(closeSpy).not.toHaveBeenCalled();
  });
});

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
    sharedMocks.getLobbyCodeFromUrl.mockReturnValue('URL22');
    sharedMocks.getEntryStep.mockReturnValue('connecting');
    sharedMocks.establishGameSession.mockResolvedValue({ ok: true });
    sharedMocks.validateRoomCode.mockResolvedValue({ ok: true });
    sharedMocks.resolveLobbyCode.mockResolvedValue('ROOM2');
  });

  it('routes inbound frames through the message queue', async () => {
    await connectWebSocket();
    const buf = new ArrayBuffer(4);
    MockWebSocket.lastInstance?.onmessage?.({ data: buf } as MessageEvent);
    drainPendingMessages(1);
    expect(handleBinaryMessage).toHaveBeenCalledWith(buf);
  });

  it('shows error when session establishment fails', async () => {
    sharedMocks.establishGameSession.mockResolvedValueOnce({
      ok: false,
      reason: 'network',
    } as import('../shared/network/network.js').SessionResult);
    await connectWebSocket();
    expect(showConnectionErrorUI).toHaveBeenCalledWith('auth failed', { showActions: true });
  });

  it('calls onLobbyCodeReady after session succeeds when URL has code', async () => {
    // 回归：onLobbyCodeReady 必须在 establishGameSession 成功后才调用，
    // 否则失败时 entryStep 已推进到 'nickname'，错误面板会被 nickname-setup-screen 盖住。
    const order: string[] = [];
    sharedMocks.onLobbyCodeReady.mockImplementation((code: string) => {
      order.push(`ready:${code}`);
    });
    sharedMocks.establishGameSession.mockImplementation(async () => {
      order.push('session');
      return { ok: true as const };
    });
    await connectWebSocket();
    expect(order[0]).toBe('session');
    expect(order[1]).toBe('ready:URL22');
  });

  it('does not call onLobbyCodeReady when session fails', async () => {
    // 失败时 entryStep 必须保持 'connecting'，不能调用 onLobbyCodeReady 推进到 'nickname'
    sharedMocks.establishGameSession.mockResolvedValueOnce({
      ok: false,
      reason: 'network',
    } as import('../shared/network/network.js').SessionResult);
    await connectWebSocket();
    expect(sharedMocks.onLobbyCodeReady).not.toHaveBeenCalled();
    expect(showConnectionErrorUI).toHaveBeenCalled();
  });

  it('does not call onLobbyCodeReady again when already past connecting', async () => {
    sharedMocks.getEntryStep.mockReturnValue('waiting');
    await connectWebSocket();
    expect(sharedMocks.onLobbyCodeReady).not.toHaveBeenCalled();
  });

  it('shows error when room validation fails', async () => {
    sharedMocks.validateRoomCode.mockResolvedValueOnce({ ok: false, reason: 'not_found' });
    await connectWebSocket();
    expect(showConnectionErrorUI).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ title: '无法进入房间' }),
    );
  });

  it('shows error when lobby match returns null', async () => {
    sharedMocks.getLobbyCodeFromUrl.mockReturnValueOnce(null);
    sharedMocks.resolveLobbyCode.mockResolvedValueOnce(null);
    await connectWebSocket();
    expect(showConnectionErrorUI).toHaveBeenCalledWith(
      '无法连接到游戏服务器，请稍后重试',
      expect.objectContaining({ showActions: true }),
    );
  });

  it('publishes matched lobby code before opening socket', async () => {
    sharedMocks.getLobbyCodeFromUrl.mockReturnValueOnce(null);
    sharedMocks.resolveLobbyCode.mockResolvedValueOnce('MATCH2');
    await connectWebSocket();
    expect(sharedMocks.onLobbyCodeReady).toHaveBeenCalledWith('MATCH2');
  });

  it('uses fresh match sessionStorage without re-validating room', async () => {
    sessionStorage.setItem('uppy-fresh-match', 'URL22');
    await connectWebSocket();
    expect(sharedMocks.validateRoomCode).not.toHaveBeenCalled();
    expect(wasRoomPreChecked()).toBe(true);
    sessionStorage.clear();
    setRoomPreChecked(false);
  });

  it('ignores connect while another connect is in flight', async () => {
    let resolveSession: (value: import('../shared/network/network.js').SessionResult) => void = () => {};
    sharedMocks.establishGameSession.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSession = resolve;
        }),
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
    sharedMocks.getLobbyCodeFromUrl.mockReturnValue('OTHER');
    sharedMocks.getEntryStep.mockReturnValue('waiting');
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
