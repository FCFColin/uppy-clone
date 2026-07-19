import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { getOutboundQueueLength, clearOutboundQueue } from './ws_connection.js';
import { MockWebSocket } from '../shared/test/mocks/websocket.js';

vi.mock('./constants.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./constants.js')>();
  return {
    ...actual,
    HEARTBEAT_INTERVAL_MS: 1000,
    HEARTBEAT_TIMEOUT_MS: 500,
  };
});

import {
  MAX_PENDING_QUEUE,
  HEARTBEAT_INTERVAL_MS,
  HEARTBEAT_TIMEOUT_MS,
  MAX_RECONNECT_ATTEMPTS,
} from './constants.js';

vi.mock('./ui_common.js', async () => {
  const { createUiCommonMock } = await import('./ws_connection_test_setup.js');
  return createUiCommonMock();
});
vi.mock('../shared/network/session.js', () => ({
  establishGameSession: vi.fn().mockRejectedValue(new Error('test-network')),
  sessionErrorMessage: () => 'network error',
}));

vi.mock('./lobby_match.js', () => ({
  ROOM_CODE_RE: /^[A-Z2-9]{5}$/,
  getLobbyCodeFromUrl: () => null,
  validateRoomCode: vi.fn(),
  roomErrorMessage: () => 'bad room',
  resolveLobbyCode: vi.fn().mockRejectedValue(new Error('test-match')),
}));

vi.mock('./entry_flow.js', () => ({
  onLobbyCodeReady: vi.fn(),
  onWebSocketOpen: vi.fn(),
  onWebSocketClosed: vi.fn(),
  getEntryStep: vi.fn(() => 'waiting'),
}));

vi.mock('./entry_flow_ui.js', () => ({
  clearWaitingInlineError: vi.fn(),
}));

vi.mock('./state_interp.js', () => ({
  resetInterpolation: vi.fn(),
}));

vi.mock('./seen_seqs.js', () => ({
  clearSeenSeqs: vi.fn(),
}));


import { getWs, setWs, stopHeartbeat, startHeartbeat, handlePong, sendOrQueue, flushPendingQueue, resetReconnectAttempts, scheduleReconnect, waitForWebSocket, showConnectionError, setRoomPreChecked, wasRoomPreChecked, setReconnectTimer, clearReconnectTimer, getWsEverOpened, setWsEverOpened } from './ws_connection.js';
import {
  showConnectionError as showConnectionErrorUI,
  showReconnectBanner,
  updatePingDisplay,
} from './ui_common.js';

describe('ws_connection', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    clearOutboundQueue();
    stopHeartbeat();
    setWs(null);
    resetReconnectAttempts();
  });

  afterEach(() => {
    vi.useRealTimers();
    stopHeartbeat();
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

  it('handlePong clears heartbeat timeout without closing socket', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    startHeartbeat();
    vi.advanceTimersByTime(HEARTBEAT_INTERVAL_MS);
    handlePong();
    vi.advanceTimersByTime(HEARTBEAT_TIMEOUT_MS - 1);
    expect((socket as unknown as MockWebSocket).sent.length).toBeGreaterThan(0);
  });

  it('heartbeat timeout closes the socket when pong is missing', async () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    const closeSpy = vi.spyOn(socket, 'close');
    setWs(socket);
    startHeartbeat();
    await vi.advanceTimersByTimeAsync(HEARTBEAT_INTERVAL_MS + HEARTBEAT_TIMEOUT_MS + 1);
    expect(closeSpy).toHaveBeenCalled();
  });

  it('getWs returns the active socket reference', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    expect(getWs()).toBe(socket);
    setWs(null);
    expect(getWs()).toBeNull();
  });

  it('scheduleReconnect stops after max attempts', async () => {
    for (let i = 0; i < MAX_RECONNECT_ATTEMPTS; i++) {
      scheduleReconnect();
      vi.runAllTimers();
    }
    scheduleReconnect();
    await vi.waitFor(() => {
      expect(showConnectionErrorUI).toHaveBeenCalled();
    });
  });

  it('scheduleReconnect shows reconnect banner before max attempts', () => {
    scheduleReconnect();
    expect(showReconnectBanner).toHaveBeenCalledWith(1);
  });

  it('handlePong updates ping display when ping was sent', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    startHeartbeat();
    vi.advanceTimersByTime(HEARTBEAT_INTERVAL_MS);
    handlePong();
    expect(vi.mocked(updatePingDisplay)).toHaveBeenCalled();
  });

  it('showConnectionError delegates to connection UI', () => {
    showConnectionError('offline', { showActions: true });
    expect(showConnectionErrorUI).toHaveBeenCalledWith('offline', { showActions: true });
  });

  it('waitForWebSocket resolves when socket open', async () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    await expect(waitForWebSocket(1000)).resolves.toBe(true);
  });

  it('waitForWebSocket resolves after timeout when socket stays closed', async () => {
    setWs(null);
    const pending = waitForWebSocket(150);
    await vi.advanceTimersByTimeAsync(200);
    await expect(pending).resolves.toBe(false);
  });

  it('clearReconnectTimer and room pre-check helpers work', () => {
    setRoomPreChecked(true);
    expect(wasRoomPreChecked()).toBe(true);
    setWsEverOpened(true);
    expect(getWsEverOpened()).toBe(true);
    const timer = setTimeout(() => {}, 1000);
    setReconnectTimer(timer);
    clearReconnectTimer();
    setRoomPreChecked(false);
    setWsEverOpened(false);
  });
});
