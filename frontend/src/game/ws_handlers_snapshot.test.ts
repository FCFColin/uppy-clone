import { describe, it, expect, vi, beforeEach } from 'vitest';

function buildMinimalSnapshot(phaseCode: number, timestamp = 100): ArrayBuffer {
  const buf = new ArrayBuffer(40);
  const dv = new DataView(buf);
  dv.setUint8(0, 0x10);
  let o = 1;
  dv.setUint32(o, timestamp, true); o += 4;
  dv.setUint32(o, 42, true); o += 4;
  dv.setUint8(o, phaseCode); o += 1;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setFloat32(o, 0.6, true); o += 4;
  dv.setFloat32(o, 0.0, true); o += 4;
  dv.setFloat32(o, 0.0, true); o += 4;
  dv.setUint8(o, 0); o += 1;
  dv.setUint8(o, 0); o += 1;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setFloat32(o, 0.5, true); o += 4;
  dv.setUint16(o, 0, true); o += 2;
  dv.setUint8(o, 0);
  return buf;
}

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
  it('handles parse errors without throwing', () => {
    const bad = new ArrayBuffer(37);
    const dv = new DataView(bad);
    dv.setUint8(0, 0x10);
    dv.setUint32(1, 1, true);
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(() => handleSnapshot(dv)).not.toThrow();
    err.mockRestore();
  });
});
