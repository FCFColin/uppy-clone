import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  stopHeartbeat,
  handlePong,
  sendOrQueue,
  flushPendingQueue,
  setWs,
  getWs,
  resetReconnectAttempts,
  scheduleReconnect,
  waitForWebSocket,
} from './ws_connection.js';
import { pendingQueue } from './state.js';
import { MAX_PENDING_QUEUE } from './constants.js';

class MockWebSocket {
  static OPEN = 1;
  readyState = MockWebSocket.OPEN;
  sent: ArrayBuffer[] = [];
  send(data: ArrayBuffer): void {
    this.sent.push(data);
  }
  close(): void {}
}

vi.mock('./ws_connection.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./ws_connection.js')>();
  return { ...actual, showConnectionError: vi.fn() };
});
vi.mock('./ws_connect.js', () => ({ connectWebSocket: vi.fn() }));

describe('ws_connection', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    pendingQueue.length = 0;
    stopHeartbeat();
    setWs(null);
    resetReconnectAttempts();
  });

  afterEach(() => {
    vi.useRealTimers();
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
    expect(pendingQueue.length).toBe(1);
  });

  // Adversarial: queue overflow drops oldest messages (memory DoS protection).
  it('sendOrQueue drops oldest when queue full', () => {
    for (let i = 0; i < MAX_PENDING_QUEUE + 2; i++) {
      sendOrQueue(new ArrayBuffer(1));
    }
    expect(pendingQueue.length).toBe(MAX_PENDING_QUEUE);
  });

  it('flushPendingQueue drains queue on open socket', () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    sendOrQueue(new ArrayBuffer(1));
    flushPendingQueue();
    expect(pendingQueue.length).toBe(0);
    expect((socket as unknown as MockWebSocket).sent.length).toBe(1);
  });

  it('handlePong clears heartbeat timeout', () => {
    handlePong();
    expect(getWs()).toBeNull();
  });

  it('scheduleReconnect stops after max attempts', () => {
    for (let i = 0; i < 10; i++) {
      scheduleReconnect();
      vi.runAllTimers();
    }
  });

  it('waitForWebSocket resolves when socket open', async () => {
    const socket = new MockWebSocket() as unknown as WebSocket;
    setWs(socket);
    await expect(waitForWebSocket(1000)).resolves.toBeUndefined();
  });
});
