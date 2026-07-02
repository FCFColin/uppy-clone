import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('./ws_connect.js', () => ({ connectWebSocket: vi.fn() }));
vi.mock('./connection_ui.js', () => ({
  hideReconnectBanner: vi.fn(),
  showReconnectBanner: vi.fn(),
  updatePingDisplay: vi.fn(),
  showConnectionError: vi.fn(),
}));

vi.mock('./constants.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./constants.js')>();
  return {
    ...actual,
    HEARTBEAT_INTERVAL_MS: 1000,
    HEARTBEAT_TIMEOUT_MS: 1500,
  };
});

import { HEARTBEAT_INTERVAL_MS } from './constants.js';
import { startHeartbeat, setWs, stopHeartbeat } from './ws_connection.js';

class MockWebSocket {
  static OPEN = 1;
  readyState = MockWebSocket.OPEN;
  sent: ArrayBuffer[] = [];
  close = vi.fn();
  send(data: ArrayBuffer): void {
    this.sent.push(data);
  }
}

describe('ws_connection heartbeat reset', () => {
  beforeEach(() => {
    vi.useFakeTimers();
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
