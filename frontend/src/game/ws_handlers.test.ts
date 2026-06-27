import { describe, it, expect, vi, beforeEach } from 'vitest';
import { handleBinaryMessage } from './ws_handlers.js';
import { MSG_TYPE } from './constants.js';

vi.mock('./ws_handlers_snapshot.js', () => ({
  handleSnapshot: vi.fn(),
}));
vi.mock('./ws_handlers_phase.js', () => ({
  handleGameStateChange: vi.fn(),
  handleRestartStatus: vi.fn(),
}));
vi.mock('./ws_handlers_events.js', () => ({
  handlePlayerJoin: vi.fn(),
  handlePlayerLeave: vi.fn(),
  handleTapAccepted: vi.fn(),
  handleTapRejected: vi.fn(),
}));
vi.mock('./ws_connection.js', () => ({
  handlePong: vi.fn(),
}));

import { handleSnapshot } from './ws_handlers_snapshot.js';
import { handlePong } from './ws_connection.js';

describe('handleBinaryMessage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('routes snapshot messages', () => {
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, MSG_TYPE.SNAPSHOT);
    handleBinaryMessage(buf);
    expect(handleSnapshot).toHaveBeenCalledOnce();
  });

  it('routes pong messages', () => {
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, MSG_TYPE.PONG);
    handleBinaryMessage(buf);
    expect(handlePong).toHaveBeenCalledOnce();
  });

  // Adversarial: unknown opcode must not throw (DoS via malformed frames).
  it('ignores unknown message types', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const buf = new ArrayBuffer(1);
    new DataView(buf).setUint8(0, 0xff);
    expect(() => handleBinaryMessage(buf)).not.toThrow();
    expect(warn).toHaveBeenCalled();
  });
});
