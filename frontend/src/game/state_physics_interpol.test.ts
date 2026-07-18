import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { state } from './state.js';
import {
  updateInterpolation,
  resetInterpolation,
  freezeInterpolation,
  getInterpolatedBalloon,
  getInterpolatedGhost,
  getInterpolatedBird,
  getInterpState,
  commitRenderedState,
} from './state_interp.js';
import { isDuplicateSeq, clearSeenSeqs, getSeenSeqsSize } from './seen_seqs.js';
import { resetClientState } from './state_interp.js';
import { MAX_SEEN_SEQS } from './constants.js';
import { PHYSICS } from '../shared/game/constants.js';

const TICK_MS = PHYSICS.TICK_INTERVAL;

describe('Physics interpolation - resetInterpolation', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetInterpolation();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('clears prevBalloon and prevGhost to null', () => {
    expect(getInterpState().prevBalloon).toBeNull();
    expect(getInterpState().prevGhost).toBeNull();
  });

  it('resets currBalloon to the default position', () => {
    expect(getInterpState().currBalloon).toEqual({ x: 0.5, y: 0.95 });
  });

  it('resets currGhost to the inactive default', () => {
    expect(getInterpState().currGhost).toEqual({ x: 0.5, y: 0.5, active: false });
  });
});

describe('Physics interpolation - updateInterpolation (first call)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    resetInterpolation();
    resetClientState();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('initializes prev and curr to the current state values on first call', () => {
    state.balloon.x = 0.3;
    state.balloon.y = 0.7;
    state.ghost.x = 0.4;
    state.ghost.y = 0.6;
    state.ghost.active = true;

    updateInterpolation(0);

    expect(getInterpState().prevBalloon).toEqual({ x: 0.3, y: 0.7 });
    expect(getInterpState().currBalloon).toEqual({ x: 0.3, y: 0.7 });
    expect(getInterpState().prevGhost).toEqual({ x: 0.4, y: 0.6, active: true });
    expect(getInterpState().currGhost).toEqual({ x: 0.4, y: 0.6, active: true });
  });
});

describe('Physics interpolation - getInterpolatedBalloon', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    resetInterpolation();
    resetClientState();
    state.balloon.x = 0.1;
    state.balloon.y = 0.2;
    updateInterpolation(0);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns currBalloon when prevBalloon is null', () => {
    resetInterpolation();
    expect(getInterpolatedBalloon()).toEqual({ x: 0.5, y: 0.95 });
  });

  it('linearly interpolates between prev and curr as tick time advances', () => {
    state.balloon.x = 0.14;
    state.balloon.y = 0.24;
    updateInterpolation(1);

    let r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.1, 1);
    expect(r.y).toBeCloseTo(0.2, 1);

    vi.advanceTimersByTime(TICK_MS / 2);
    r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.12, 1);
    expect(r.y).toBeCloseTo(0.22, 1);

    vi.advanceTimersByTime(TICK_MS / 2);
    r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.14, 1);
    expect(r.y).toBeCloseTo(0.24, 1);
  });

  it('allows velocity extrapolation once elapsed exceeds one tick interval', () => {
    state.balloon.x = 0.14;
    state.balloon.y = 0.24;
    state.balloon.vx = 0.01;
    state.balloon.vy = 0.01;
    updateInterpolation(1);

    vi.advanceTimersByTime(TICK_MS * 1.1);
    const r = getInterpolatedBalloon();
    expect(r.x).toBeGreaterThan(0.14);
    expect(r.y).toBeGreaterThan(0.24);
  });

  it('does not snap backward when a new snapshot arrives after extrapolation', () => {
    state.balloon.x = 0.14;
    state.balloon.y = 0.24;
    state.balloon.vx = 0.01;
    state.balloon.vy = 0.01;
    updateInterpolation(1);

    vi.advanceTimersByTime(TICK_MS * 1.1);
    const beforeSnap = getInterpolatedBalloon();
    commitRenderedState();

    state.balloon.x = 0.16;
    state.balloon.y = 0.26;
    updateInterpolation(2);
    const afterSnap = getInterpolatedBalloon();

    expect(afterSnap.x).toBeGreaterThanOrEqual(beforeSnap.x - 0.02);
    expect(afterSnap.y).toBeGreaterThanOrEqual(beforeSnap.y - 0.02);
  });

  it('handles the edge case of zero delta (same position)', () => {
    state.balloon.x = 0.1;
    state.balloon.y = 0.2;
    updateInterpolation(1);

    vi.advanceTimersByTime(TICK_MS / 2);
    const r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.1, 8);
    expect(r.y).toBeCloseTo(0.2, 8);
  });

  it('snaps (no smoothing) when movement exceeds the teleport threshold', () => {
    state.balloon.x = 0.6;
    state.balloon.y = 0.7;
    updateInterpolation(1);

    expect(getInterpolatedBalloon()).toEqual({ x: 0.6, y: 0.7 });
    vi.advanceTimersByTime(TICK_MS / 2);
    expect(getInterpolatedBalloon()).toEqual({ x: 0.6, y: 0.7 });
  });
});

describe('Physics interpolation - getInterpolatedGhost', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    resetInterpolation();
    resetClientState();
    state.balloon.x = 0.5;
    state.balloon.y = 0.5;
    updateInterpolation(0);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns null when the ghost is inactive', () => {
    state.ghost.active = false;
    updateInterpolation(1);
    expect(getInterpolatedGhost()).toBeNull();
  });

  it('returns currGhost when prevGhost is null (first activation)', () => {
    state.ghost.active = true;
    state.ghost.x = 0.2;
    state.ghost.y = 0.3;
    updateInterpolation(1);
    expect(getInterpolatedGhost()).toEqual({ x: 0.2, y: 0.3, active: true });
  });

  it('linearly interpolates the ghost position between snapshots', () => {
    state.ghost.active = true;
    state.ghost.x = 0.1;
    state.ghost.y = 0.2;
    updateInterpolation(1);

    state.ghost.x = 0.14;
    state.ghost.y = 0.24;
    updateInterpolation(2);

    vi.advanceTimersByTime(TICK_MS / 2);
    let g = getInterpolatedGhost();
    expect(g).not.toBeNull();
    expect(g!.x).toBeCloseTo(0.12, 1);
    expect(g!.y).toBeCloseTo(0.22, 1);

    vi.advanceTimersByTime(TICK_MS / 2);
    g = getInterpolatedGhost();
    expect(g).not.toBeNull();
    expect(g!.x).toBeCloseTo(0.14, 2);
    expect(g!.y).toBeCloseTo(0.24, 2);
  });

  it('returns null again once the ghost becomes inactive', () => {
    state.ghost.active = true;
    state.ghost.x = 0.2;
    state.ghost.y = 0.3;
    updateInterpolation(1);
    expect(getInterpolatedGhost()).not.toBeNull();

    state.ghost.active = false;
    updateInterpolation(2);
    expect(getInterpolatedGhost()).toBeNull();
  });
});

describe('Physics interpolation - getInterpolatedBird', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    resetInterpolation();
    resetClientState();
    updateInterpolation(0);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns null when bird inactive', () => {
    state.bird.active = false;
    updateInterpolation(1);
    expect(getInterpolatedBird()).toBeNull();
  });

  it('interpolates active bird between ticks', () => {
    state.bird.active = true;
    state.bird.x = 0.2;
    state.bird.y = 0.3;
    updateInterpolation(1);

    state.bird.x = 0.24;
    state.bird.y = 0.34;
    updateInterpolation(2);

    vi.advanceTimersByTime(TICK_MS / 2);
    const bird = getInterpolatedBird();
    expect(bird).not.toBeNull();
    expect(bird!.x).toBeCloseTo(0.22, 1);
    expect(bird!.y).toBeCloseTo(0.32, 1);
  });

  it('uses delay-buffer interpolation for balloon, ghost, and bird', () => {
    state.balloon = { x: 0.1, y: 0.2, vx: 0.01, vy: 0.02 };
    state.ghost = { x: 0.3, y: 0.4, active: true, repelTimer: 0 };
    state.bird = { x: 0.5, y: 0.6, active: true };
    updateInterpolation(1);
    vi.advanceTimersByTime(TICK_MS);
    state.balloon = { x: 0.15, y: 0.25, vx: 0.01, vy: 0.02 };
    state.ghost = { x: 0.35, y: 0.45, active: true, repelTimer: 0 };
    state.bird = { x: 0.55, y: 0.65, active: true };
    updateInterpolation(2);
    vi.advanceTimersByTime(PHYSICS.INTERP_DELAY_MS + TICK_MS / 2);
    const balloon = getInterpolatedBalloon();
    const ghost = getInterpolatedGhost();
    const bird = getInterpolatedBird();
    expect(balloon.x).toBeGreaterThan(0.1);
    expect(ghost).not.toBeNull();
    expect(bird).not.toBeNull();
  });
});

describe('Physics interpolation - freezeInterpolation', () => {
  beforeEach(() => {
    resetInterpolation();
    resetClientState();
    state.balloon = { x: 0.2, y: 0.3, vx: 0, vy: 0 };
    state.ghost = { x: 0.4, y: 0.5, active: true, repelTimer: 0 };
    state.bird = { x: 0.6, y: 0.7, active: true };
    updateInterpolation(1);
  });

  it('pins rendered entities to the current authoritative state', () => {
    state.balloon = { x: 0.25, y: 0.35, vx: 0, vy: 0 };
    state.ghost = { x: 0.45, y: 0.55, active: true, repelTimer: 0 };
    state.bird = { x: 0.65, y: 0.75, active: true };
    freezeInterpolation();
    expect(getInterpolatedBalloon()).toEqual({ x: 0.25, y: 0.35 });
    expect(getInterpolatedGhost()).toEqual({ x: 0.45, y: 0.55, active: true });
    expect(getInterpolatedBird()).toEqual({ x: 0.65, y: 0.75, active: true });
  });
});

describe('Physics interpolation - isDuplicateSeq', () => {
  beforeEach(() => {
    clearSeenSeqs();
  });

  it('returns false for a sequence seen for the first time', () => {
    expect(isDuplicateSeq(42)).toBe(false);
  });

  it('returns true for a repeated sequence', () => {
    isDuplicateSeq(42);
    expect(isDuplicateSeq(42)).toBe(true);
  });

  it('tracks distinct sequences independently', () => {
    isDuplicateSeq(1);
    isDuplicateSeq(2);
    expect(isDuplicateSeq(1)).toBe(true);
    expect(isDuplicateSeq(2)).toBe(true);
    expect(isDuplicateSeq(3)).toBe(false);
  });

  it('evicts the oldest entries once the set exceeds MAX_SEEN_SEQS', () => {
    for (let i = 0; i < MAX_SEEN_SEQS; i += 1) {
      isDuplicateSeq(i);
    }
    expect(getSeenSeqsSize()).toBe(MAX_SEEN_SEQS);

    expect(isDuplicateSeq(MAX_SEEN_SEQS)).toBe(false);
    expect(getSeenSeqsSize()).toBe(MAX_SEEN_SEQS - Math.floor(MAX_SEEN_SEQS / 2) + 1);
    expect(isDuplicateSeq(0)).toBe(false);
    expect(isDuplicateSeq(MAX_SEEN_SEQS - 1)).toBe(true);
  });
});

describe('Physics interpolation - resetClientState', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetInterpolation();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('resets score, cooldown, ripples, balloon and wind to defaults', () => {
    state.score = 999;
    state.myCooldownEnd = 12345;
    state.ripples.push({ playerIndex: 1, x: 0.5, y: 0.5, time: 0 });
    state.balloon = { x: 0.9, y: 0.9, vx: 0.1, vy: 0.1 };
    state.wind = 0.5;
    state.hasReceivedFirstSnapshot = true;

    resetClientState();

    expect(state.score).toBe(0);
    expect(state.myCooldownEnd).toBe(0);
    expect(state.ripples).toEqual([]);
    expect(state.balloon).toEqual({ x: 0.5, y: 0.5, vx: 0, vy: 0 });
    expect(state.wind).toBe(0);
    expect(state.hasReceivedFirstSnapshot).toBe(false);
  });

  it('clears the seenSeqs set', () => {
    isDuplicateSeq(1);
    isDuplicateSeq(2);
    resetClientState();
    expect(getSeenSeqsSize()).toBe(0);
  });
});
