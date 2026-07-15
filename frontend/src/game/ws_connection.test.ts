import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { getOutboundQueueLength, clearOutboundQueue } from './ws_connection.js';
import { MockWebSocket } from '../shared/test/mocks/websocket.js';

vi.mock('./local_constants.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./local_constants.js')>();
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
} from './local_constants.js';

vi.mock('./ws_connect.js', () => ({ connectWebSocket: vi.fn() }));
vi.mock('./connection_ui.js', () => ({
  hideReconnectBanner: vi.fn(),
  showReconnectBanner: vi.fn(),
  updatePingDisplay: vi.fn(),
  showConnectionError: vi.fn(),
}));

import { getWs, setWs, stopHeartbeat, startHeartbeat, handlePong, sendOrQueue, flushPendingQueue, resetReconnectAttempts, scheduleReconnect, waitForWebSocket, showConnectionError, setRoomPreChecked, wasRoomPreChecked, setReconnectTimer, clearReconnectTimer, getWsEverOpened, setWsEverOpened } from './ws_connection.js';
import {
  showConnectionError as showConnectionErrorUI,
  showReconnectBanner,
  updatePingDisplay,
} from './connection_ui.js';

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

  it('sendOrQueue sends when socket open', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    const buf = new ArrayBuffer(1);
    sendOrQueue(buf);
    expect((socket as unknown as MockWebSocket).sent.length).toBe(1);
  });

  it('sendOrQueue queues when socket closed', () => {
    const buf = new ArrayBuffer(1);
    sendOrQueue(buf);
    expect(getOutboundQueueLength()).toBe(1);
  });

  it('sendOrQueue drops oldest when queue full', () => {
    for (let i = 0; i < MAX_PENDING_QUEUE + 2; i++) {
      sendOrQueue(new ArrayBuffer(1));
    }
    expect(getOutboundQueueLength()).toBe(MAX_PENDING_QUEUE);
  });

  it('flushPendingQueue drains queue on open socket', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(null);
    sendOrQueue(new ArrayBuffer(1));
    setWs(socket);
    flushPendingQueue();
    expect(getOutboundQueueLength()).toBe(0);
    expect((socket as unknown as MockWebSocket).sent.length).toBe(1);
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

  it('flushPendingQueue drains multiple queued messages', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(null);
    sendOrQueue(new ArrayBuffer(1));
    sendOrQueue(new ArrayBuffer(1));
    sendOrQueue(new ArrayBuffer(1));
    setWs(socket);
    flushPendingQueue();
    expect(getOutboundQueueLength()).toBe(0);
    expect((socket as unknown as MockWebSocket).sent.length).toBe(3);
  });

  it('getWs returns the active socket reference', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    expect(getWs()).toBe(socket);
    setWs(null);
    expect(getWs()).toBeNull();
  });

  it('flushPendingQueue no-ops when socket is closed', () => {
    sendOrQueue(new ArrayBuffer(1));
    flushPendingQueue();
    expect(getOutboundQueueLength()).toBe(1);
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
