import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  state,
  updateInterpolation,
  resetInterpolation,
  getInterpolatedBalloon,
  getInterpolatedGhost,
  isDuplicateSeq,
  seenSeqs,
  resetClientState,
  getInterpState,
} from './state.js';
import { MAX_SEEN_SEQS } from './constants.js';

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
    vi.setSystemTime(1000);
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

    updateInterpolation();

    expect(getInterpState().prevBalloon).toEqual({ x: 0.3, y: 0.7 });
    expect(getInterpState().currBalloon).toEqual({ x: 0.3, y: 0.7 });
    expect(getInterpState().prevGhost).toEqual({ x: 0.4, y: 0.6, active: true });
    expect(getInterpState().currGhost).toEqual({ x: 0.4, y: 0.6, active: true });
  });
});

describe('Physics interpolation - getInterpolatedBalloon', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(1000);
    resetInterpolation();
    resetClientState();
    // First snapshot at position (0.1, 0.2)
    state.balloon.x = 0.1;
    state.balloon.y = 0.2;
    updateInterpolation(); // prev = curr = {0.1, 0.2}, prevTime=934, currTime=1000
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns currBalloon when prevBalloon is null', () => {
    resetInterpolation();
    // After reset prevBalloon is null and currBalloon is the default {0.5, 0.95}
    expect(getInterpolatedBalloon()).toEqual({ x: 0.5, y: 0.95 });
  });

  it('linearly interpolates between prev and curr as time advances', () => {
    // Move by 0.04 on each axis (under the 0.05 teleport threshold) so interpolation applies.
    state.balloon.x = 0.14;
    state.balloon.y = 0.24;
    vi.setSystemTime(1100);
    updateInterpolation(); // prev={0.1,0.2}, curr={0.14,0.24}, prevTime=1000, currTime=1100

    // alpha = 0 -> returns prev
    vi.setSystemTime(1100);
    let r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.1, 8);
    expect(r.y).toBeCloseTo(0.2, 8);

    // alpha = 0.5 -> midpoint
    vi.setSystemTime(1150);
    r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.12, 8);
    expect(r.y).toBeCloseTo(0.22, 8);

    // alpha = 1 -> returns curr
    vi.setSystemTime(1200);
    r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.14, 8);
    expect(r.y).toBeCloseTo(0.24, 8);
  });

  it('clamps alpha at 1 once elapsed exceeds the snapshot interval', () => {
    state.balloon.x = 0.14;
    state.balloon.y = 0.24;
    vi.setSystemTime(1100);
    updateInterpolation(); // interval = 100

    // Well past the snapshot interval -> clamped to curr
    vi.setSystemTime(1300);
    const r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.14, 8);
    expect(r.y).toBeCloseTo(0.24, 8);
  });

  it('handles the edge case of zero delta (same position)', () => {
    // Same position as the first snapshot -> delta is 0, no teleport, prev == curr.
    state.balloon.x = 0.1;
    state.balloon.y = 0.2;
    vi.setSystemTime(1100);
    updateInterpolation();

    vi.setSystemTime(1150);
    const r = getInterpolatedBalloon();
    expect(r.x).toBeCloseTo(0.1, 8);
    expect(r.y).toBeCloseTo(0.2, 8);
  });

  it('snaps (no smoothing) when movement exceeds the teleport threshold', () => {
    // prev = curr = {0.1, 0.2} from beforeEach. Move beyond 0.05 threshold.
    state.balloon.x = 0.6;
    state.balloon.y = 0.7;
    vi.setSystemTime(1100);
    updateInterpolation(); // teleport: prev snaps to {0.6, 0.7}, curr = {0.6, 0.7}

    // Because prev == curr, interpolation returns the new position immediately at any alpha.
    vi.setSystemTime(1100);
    expect(getInterpolatedBalloon()).toEqual({ x: 0.6, y: 0.7 });
    vi.setSystemTime(1150);
    expect(getInterpolatedBalloon()).toEqual({ x: 0.6, y: 0.7 });
  });
});

describe('Physics interpolation - getInterpolatedGhost', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(1000);
    resetInterpolation();
    resetClientState();
    // Keep balloon stable so it does not interfere with ghost logic.
    state.balloon.x = 0.5;
    state.balloon.y = 0.5;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns null when the ghost is inactive', () => {
    state.ghost.active = false;
    updateInterpolation();
    expect(getInterpolatedGhost()).toBeNull();
  });

  it('returns currGhost when prevGhost is null (first activation)', () => {
    state.ghost.active = true;
    state.ghost.x = 0.2;
    state.ghost.y = 0.3;
    updateInterpolation(); // first call: prev = curr
    expect(getInterpolatedGhost()).toEqual({ x: 0.2, y: 0.3, active: true });
  });

  it('linearly interpolates the ghost position between snapshots', () => {
    // First snapshot: ghost at (0.1, 0.2)
    state.ghost.active = true;
    state.ghost.x = 0.1;
    state.ghost.y = 0.2;
    updateInterpolation();

    // Second snapshot: ghost at (0.14, 0.24), 100ms later (delta 0.04, under threshold)
    state.ghost.x = 0.14;
    state.ghost.y = 0.24;
    vi.setSystemTime(1100);
    updateInterpolation(); // prev={0.1,0.2}, curr={0.14,0.24}

    vi.setSystemTime(1150); // alpha = 0.5
    let g = getInterpolatedGhost();
    expect(g).not.toBeNull();
    expect(g!.x).toBeCloseTo(0.12, 8);
    expect(g!.y).toBeCloseTo(0.22, 8);

    vi.setSystemTime(1200); // alpha = 1
    g = getInterpolatedGhost();
    expect(g).not.toBeNull();
    expect(g!.x).toBeCloseTo(0.14, 8);
    expect(g!.y).toBeCloseTo(0.24, 8);
  });

  it('returns null again once the ghost becomes inactive', () => {
    state.ghost.active = true;
    state.ghost.x = 0.2;
    state.ghost.y = 0.3;
    updateInterpolation();
    expect(getInterpolatedGhost()).not.toBeNull();

    state.ghost.active = false;
    vi.setSystemTime(1100);
    updateInterpolation();
    expect(getInterpolatedGhost()).toBeNull();
  });
});

describe('Physics interpolation - isDuplicateSeq', () => {
  beforeEach(() => {
    seenSeqs.clear();
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
    // Fill up to the boundary without triggering eviction.
    for (let i = 0; i < MAX_SEEN_SEQS; i += 1) {
      isDuplicateSeq(i);
    }
    expect(seenSeqs.size).toBe(MAX_SEEN_SEQS);

    // One more unique seq triggers eviction of the oldest half.
    expect(isDuplicateSeq(MAX_SEEN_SEQS)).toBe(false);
    expect(seenSeqs.size).toBe(MAX_SEEN_SEQS - Math.floor(MAX_SEEN_SEQS / 2) + 1);

    // An evicted (oldest) seq should no longer be considered a duplicate.
    expect(isDuplicateSeq(0)).toBe(false);
    // A still-present (newer) seq should remain a duplicate.
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
    seenSeqs.add(1);
    seenSeqs.add(2);
    resetClientState();
    expect(seenSeqs.size).toBe(0);
  });
});
