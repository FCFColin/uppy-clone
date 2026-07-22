import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { state } from './state.js';
import {
  balloonBuffer,
  birdBuffer,
  clearAnchorBuffers,
  findAnchorIndex,
  ghostBuffer,
  MAX_SNAPSHOT_BUFFER,
  pushAnchors,
  TICK_MS,
  tryBalloonFromDelayBuffer,
  tryEntityFromDelayBuffer,
  INTERP_DELAY_MS,
} from './interp_buffers.js';

describe('interp_buffers', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(10_000);
    clearAnchorBuffers();
    state.balloon = { x: 0.1, y: 0.2, vx: 0.01, vy: 0.02 };
    state.ghost = { x: 0.3, y: 0.4, active: true, repelTimer: 0 };
    state.bird = { x: 0.5, y: 0.6, active: true };
  });

  afterEach(() => {
    vi.useRealTimers();
    clearAnchorBuffers();
  });

  it('pushAnchors trims buffers to MAX_SNAPSHOT_BUFFER', () => {
    for (let i = 0; i < MAX_SNAPSHOT_BUFFER + 3; i++) {
      pushAnchors(i);
    }
    expect(balloonBuffer.length).toBe(MAX_SNAPSHOT_BUFFER);
    expect(ghostBuffer.length).toBe(MAX_SNAPSHOT_BUFFER);
    expect(birdBuffer.length).toBe(MAX_SNAPSHOT_BUFFER);
  });

  it('findAnchorIndex returns the latest anchor at or before render time', () => {
    balloonBuffer.push(
      { tick: 1, receivedAt: 100, x: 0, y: 0, vx: 0, vy: 0 },
      { tick: 2, receivedAt: 200, x: 0, y: 0, vx: 0, vy: 0 },
      { tick: 3, receivedAt: 300, x: 0, y: 0, vx: 0, vy: 0 },
    );
    expect(findAnchorIndex(balloonBuffer, 250)).toBe(1);
    expect(findAnchorIndex(balloonBuffer, 50)).toBe(-1);
  });

  it('tryBalloonFromDelayBuffer extrapolates with a single anchor', () => {
    pushAnchors(1);
    vi.setSystemTime(10_000 + INTERP_DELAY_MS + TICK_MS);
    const point = tryBalloonFromDelayBuffer();
    expect(point).not.toBeNull();
    expect(point!.x).toBeGreaterThan(0.1);
    expect(point!.y).toBeGreaterThan(0.2);
  });

  it('tryBalloonFromDelayBuffer interpolates between two anchors', () => {
    balloonBuffer.push(
      { tick: 1, receivedAt: 1000, x: 0.1, y: 0.2, vx: 0, vy: 0 },
      { tick: 2, receivedAt: 2000, x: 0.5, y: 0.6, vx: 0, vy: 0 },
    );
    vi.setSystemTime(1500 + INTERP_DELAY_MS);
    expect(tryBalloonFromDelayBuffer()!.x).toBeCloseTo(0.3, 5);
    expect(tryBalloonFromDelayBuffer()!.y).toBeCloseTo(0.4, 5);
  });

  it('tryBalloonFromDelayBuffer handles non-monotonic anchor times', () => {
    balloonBuffer.push(
      { tick: 2, receivedAt: 2000, x: 0.5, y: 0.6, vx: 0, vy: 0 },
      { tick: 1, receivedAt: 1500, x: 0.1, y: 0.2, vx: 0, vy: 0 },
    );
    vi.setSystemTime(2000 + INTERP_DELAY_MS);
    const point = tryBalloonFromDelayBuffer();
    const ticks = (2000 - 1500) / TICK_MS;
    const expectedY = 0.2 - 0.5 * 0.0005 * ticks * ticks;
    expect(point!.x).toBeCloseTo(0.1, 5);
    expect(point!.y).toBeCloseTo(expectedY, 5);
  });

  it('tryBalloonFromDelayBuffer extrapolates past the end anchor', () => {
    balloonBuffer.push(
      { tick: 1, receivedAt: 1000, x: 0.1, y: 0.2, vx: 0, vy: 0 },
      { tick: 2, receivedAt: 2000, x: 0.5, y: 0.6, vx: 0.02, vy: 0.03 },
    );
    vi.setSystemTime(2500 + INTERP_DELAY_MS);
    const point = tryBalloonFromDelayBuffer();
    expect(point!.x).toBeGreaterThan(0.5);
    expect(point!.y).toBeGreaterThan(0.6);
  });

  it('tryEntityFromDelayBuffer returns b position when span is non-positive or zero', () => {
    ghostBuffer.push(
      { tick: 2, receivedAt: 2000, x: 0.5, y: 0.6, active: true },
      { tick: 1, receivedAt: 1500, x: 0.1, y: 0.2, active: true },
    );
    expect(tryEntityFromDelayBuffer(ghostBuffer, 2000)).toEqual({ x: 0.1, y: 0.2 });
    ghostBuffer.length = 0;
    ghostBuffer.push(
      { tick: 1, receivedAt: 1500, x: 0.1, y: 0.2, active: true },
      { tick: 2, receivedAt: 1500, x: 0.5, y: 0.6, active: true },
    );
    expect(tryEntityFromDelayBuffer(ghostBuffer, 1500)).toEqual({ x: 0.5, y: 0.6 });
  });

  it('tryEntityFromDelayBuffer returns null for inactive first anchor, holds position when next anchor is inactive', () => {
    ghostBuffer.push(
      { tick: 1, receivedAt: 100, x: 0.1, y: 0.2, active: false },
      { tick: 2, receivedAt: 200, x: 0.3, y: 0.4, active: true },
    );
    expect(tryEntityFromDelayBuffer(ghostBuffer, 150)).toBeNull();
    ghostBuffer.length = 0;
    ghostBuffer.push(
      { tick: 1, receivedAt: 100, x: 0.1, y: 0.2, active: true },
      { tick: 2, receivedAt: 200, x: 0.9, y: 0.9, active: false },
    );
    expect(tryEntityFromDelayBuffer(ghostBuffer, 150)).toEqual({ x: 0.1, y: 0.2 });
  });

  it('tryEntityFromDelayBuffer interpolates active entities', () => {
    birdBuffer.push(
      { tick: 1, receivedAt: 100, x: 0.1, y: 0.2, active: true },
      { tick: 2, receivedAt: 200, x: 0.5, y: 0.6, active: true },
    );
    const point = tryEntityFromDelayBuffer(birdBuffer, 150);
    expect(point!.x).toBeCloseTo(0.3, 5);
    expect(point!.y).toBeCloseTo(0.4, 5);
  });

  it('returns null when render time precedes all anchors', () => {
    expect(tryEntityFromDelayBuffer([], 100)).toBeNull();
    ghostBuffer.push({ tick: 1, receivedAt: 1000, x: 0.1, y: 0.2, active: true });
    expect(tryEntityFromDelayBuffer(ghostBuffer, 50)).toBeNull();
    expect(tryBalloonFromDelayBuffer()).toBeNull();
  });
});
