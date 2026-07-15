import { vi } from 'vitest';

/**
 * Shared MockWebSocket for tests that need a fake WebSocket instance.
 *
 * Consolidates three previously duplicated inline implementations from
 * ws_connection.test.ts, ws_connection_heartbeat.test.ts, and ws_connect.test.ts.
 *
 * Features:
 * - Static constants matching the WebSocket spec (CONNECTING/OPEN/CLOSED)
 * - Event callback properties (onopen/onmessage/onclose/onerror)
 * - send() is a vi.fn spy that also records sent frames in `sent[]`
 * - close() is a vi.fn spy
 * - Static `lastInstance` tracks the most recently created socket
 *   (replaces per-file `lastSocket` module-level variables)
 */
export class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 3;
  static lastInstance: MockWebSocket | null = null;

  binaryType = 'arraybuffer';
  readyState = MockWebSocket.OPEN;
  url = '';
  onopen: (() => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  sent: ArrayBuffer[] = [];

  constructor(url = '') {
    this.url = url;
    MockWebSocket.lastInstance = this;
  }

  send = vi.fn((data: ArrayBuffer) => {
    this.sent.push(data);
  });

  close = vi.fn();
}
