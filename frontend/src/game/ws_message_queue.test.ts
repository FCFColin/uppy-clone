import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./ws_handlers.js', () => ({
  handleBinaryMessage: vi.fn(),
}));

import { handleBinaryMessage } from './ws_handlers.js';
import { enqueueBinaryMessage, drainPendingMessages } from './ws_message_queue.js';

describe('ws_message_queue', () => {
  beforeEach(() => {
    drainPendingMessages(64);
    vi.mocked(handleBinaryMessage).mockClear();
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

  it('drains nothing when queue is empty', () => {
    drainPendingMessages(10);
    expect(handleBinaryMessage).not.toHaveBeenCalled();
  });

  it('handles budget larger than queue size', () => {
    enqueueBinaryMessage(new Uint8Array([1]).buffer);
    enqueueBinaryMessage(new Uint8Array([2]).buffer);
    drainPendingMessages(100);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(2);
  });

  it('handles zero budget gracefully', () => {
    enqueueBinaryMessage(new Uint8Array([1]).buffer);
    drainPendingMessages(0);
    expect(handleBinaryMessage).not.toHaveBeenCalled();
  });

  it('drops excess messages beyond MAX_PENDING (32) maintaining FIFO order', () => {
    for (let i = 0; i < 35; i++) {
      enqueueBinaryMessage(new Uint8Array([i]).buffer);
    }
    drainPendingMessages(35);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(32);
    for (let i = 0; i < 32; i++) {
      expect(handleBinaryMessage).toHaveBeenNthCalledWith(i + 1, new Uint8Array([i + 3]).buffer);
    }
  });

  it('interleaves enqueue and drain correctly', () => {
    enqueueBinaryMessage(new Uint8Array([1]).buffer);
    drainPendingMessages(1);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(1);

    enqueueBinaryMessage(new Uint8Array([2]).buffer);
    enqueueBinaryMessage(new Uint8Array([3]).buffer);
    drainPendingMessages(2);
    expect(handleBinaryMessage).toHaveBeenCalledTimes(3);
  });
});
