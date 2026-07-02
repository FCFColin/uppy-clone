import { describe, it, expect, vi, beforeEach } from 'vitest';
import { buildMinimalSnapshot } from './test_fixtures/snapshot.js';

const mocks = vi.hoisted(() => ({
  state: {
    phase: 'waiting' as 'waiting' | 'countdown' | 'playing' | 'ended',
    score: 0,
    balloon: { x: 0.5, y: 0.5, vx: 0, vy: 0 },
    bird: { active: false, x: 0, y: 0 },
    ghost: { active: false, x: 0.5, y: 0.5, repelTimer: 0 },
    players: [] as Array<{ playerIndex: number; cooldownEndTime: number; palette: number; scoreContribution: number; nickname: string }>,
    ripples: [] as Array<{ playerIndex: number; x: number; y: number; time: number; isOptimistic?: boolean }>,
    wind: 0,
    hasReceivedFirstSnapshot: false,
    nicknameSubmitted: true,
    pendingNickname: null as string | null,
  },
  applyPhaseChange: vi.fn(() => true),
  updateInterpolation: vi.fn(),
  freezeInterpolation: vi.fn(),
  isDuplicateSeq: vi.fn(() => false),
  updateScoresOnly: vi.fn(),
}));

vi.mock('./state.js', () => ({
  state: mocks.state,
  updateInterpolation: mocks.updateInterpolation,
  freezeInterpolation: mocks.freezeInterpolation,
  isDuplicateSeq: mocks.isDuplicateSeq,
}));

vi.mock('./phase_sync.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./phase_sync.js')>();
  return {
    ...actual,
    applyPhaseChange: mocks.applyPhaseChange,
  };
});

vi.mock('./ui_update.js', () => ({
  updateScoresOnly: mocks.updateScoresOnly,
}));

vi.mock('./message_codec.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./message_codec.js')>();
  return {
    ...actual,
    decodeSnapshot: vi.fn(actual.decodeSnapshot),
  };
});

import { decodeSnapshot } from './message_codec.js';
import { handleSnapshot } from './ws_handlers_snapshot.js';

describe('handleSnapshot', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.phase = 'waiting';
    mocks.state.score = 0;
    mocks.state.hasReceivedFirstSnapshot = false;
    mocks.isDuplicateSeq.mockReturnValue(false);
  });

  it('ignores messages shorter than 37 bytes', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    handleSnapshot(new DataView(new ArrayBuffer(10)));
    expect(warn).toHaveBeenCalled();
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(false);
    warn.mockRestore();
  });

  it('parses score, balloon, and phase from a minimal snapshot', () => {
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 200)));
    expect(mocks.state.score).toBe(42);
    expect(mocks.state.balloon.x).toBeCloseTo(0.5);
    expect(mocks.state.balloon.y).toBeCloseTo(0.6);
    expect(mocks.applyPhaseChange).toHaveBeenCalledWith('playing');
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(true);
  });

  it('skips duplicate sequence timestamps', () => {
    mocks.isDuplicateSeq.mockReturnValue(true);
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 300)));
    expect(mocks.state.score).toBe(0);
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  // Adversarial: malformed trailing data must not crash the client.
  it('handles Error parse failures', () => {
    vi.mocked(decodeSnapshot).mockImplementationOnce(() => {
      throw new Error('boom');
    });
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 405)));
    expect(err).toHaveBeenCalledWith('[snapshot] parse error:', 'boom');
    err.mockRestore();
  });

  it('handles parse errors without throwing', () => {
    vi.mocked(decodeSnapshot).mockImplementationOnce(() => {
      throw 'string failure';
    });
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 404)));
    expect(err).toHaveBeenCalledWith('[snapshot] parse error:', 'string failure');
    err.mockRestore();
  });

  it('ignores undecodable snapshots', () => {
    vi.mocked(decodeSnapshot).mockReturnValueOnce(null);
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 400)));
    expect(mocks.state.hasReceivedFirstSnapshot).toBe(false);
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('skips blocked phase transitions from snapshot', () => {
    mocks.state.phase = 'countdown';
    handleSnapshot(new DataView(buildMinimalSnapshot(2, 401)));
    expect(mocks.applyPhaseChange).not.toHaveBeenCalled();
  });

  it('keeps pending nickname until player appears in roster', () => {
    mocks.state.pendingNickname = 'Ghost';
    handleSnapshot(new DataView(buildMinimalSnapshot(1, 402)));
    expect(mocks.state.pendingNickname).toBe('Ghost');
  });

  it('applies wind, ripples, pending nickname, and freezes on ended phase', () => {
    mocks.state.phase = 'ended';
    mocks.state.pendingNickname = 'Bob';
    mocks.state.ripples = [{ playerIndex: -2, x: 0.1, y: 0.1, time: 1, isOptimistic: true }];
    const nick = 'Bob';
    const nickBytes = new TextEncoder().encode(nick);
    const buf = new ArrayBuffer(80);
    const dv = new DataView(buf);
    let o = 1;
    dv.setUint32(o, 500, true); o += 4;
    dv.setUint32(o, 99, true); o += 4;
    dv.setUint8(o, 2); o += 1;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setFloat32(o, 0.6, true); o += 4;
    dv.setFloat32(o, 0, true); o += 4;
    dv.setFloat32(o, 0, true); o += 4;
    dv.setUint8(o, 0); o += 1;
    dv.setUint8(o, 0); o += 1;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setFloat32(o, 0.5, true); o += 4;
    dv.setUint16(o, 0, true); o += 2;
    dv.setUint8(o, 1); o += 1;
    dv.setUint16(o, 4, true); o += 2;
    dv.setUint32(o, 100, true); o += 4;
    dv.setUint32(o, 1, true); o += 4;
    dv.setUint32(o, 20, true); o += 4;
    dv.setUint8(o, nickBytes.length); o += 1;
    new Uint8Array(buf, o).set(nickBytes); o += nickBytes.length;
    dv.setUint8(o, 1); o += 1;
    dv.setUint16(o, 4, true); o += 2;
    dv.setFloat32(o, 0.2, true); o += 4;
    dv.setFloat32(o, 0.3, true); o += 4;
    dv.setFloat32(o, -0.4, true);

    handleSnapshot(new DataView(buf, 0, o + 4));
    expect(mocks.state.wind).toBeCloseTo(-0.4);
    expect(mocks.state.ripples.some((r) => r.playerIndex === 4)).toBe(true);
    expect(mocks.state.pendingNickname).toBeNull();
    expect(mocks.freezeInterpolation).toHaveBeenCalled();
  });
});
