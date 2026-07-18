import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockWebSocket } from '../shared/test/mocks/websocket.js';

vi.mock('./ui_common.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./ui_common.js')>();
  return {
    ...actual,
    hideReconnectBanner: vi.fn(),
    showReconnectBanner: vi.fn(),
    updatePingDisplay: vi.fn(),
    showConnectionError: vi.fn(),
  };
});

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
