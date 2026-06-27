import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./ws_handlers.js', () => ({
  handleBinaryMessage: vi.fn(),
}));

import { handleBinaryMessage } from './ws_handlers.js';
import { enqueueBinaryMessage, drainPendingMessages } from './ws_message_queue.js';

describe('ws_message_queue', () => {
  beforeEach(() => {
    vi.mocked(handleBinaryMessage).mockClear();
    drainPendingMessages(64);
  });

  it('drains enqueued messages up to budget', () => {
    enqueueBinaryMessage(new Uint8Array([1]).buffer);
    enqueueBinaryMessage(new Uint8Array([2]).buffer);
    drainPendingMessages(1);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(1);
    drainPendingMessages(1);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(2);
  });

  it('drops oldest when queue exceeds max', () => {
    for (let i = 0; i < 40; i++) {
      enqueueBinaryMessage(new Uint8Array([i]).buffer);
    }
    drainPendingMessages(32);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(32);
  });
});
