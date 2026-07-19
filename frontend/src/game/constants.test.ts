import { describe, it, expect } from 'vitest';
import { END_REASON } from '../shared/game/constants.js';
import { endReasonLabel } from './constants.js';

describe('END_REASON constants', () => {
  it('has correct integer values', () => {
    expect(END_REASON.NONE).toBe(0);
    expect(END_REASON.GROUND).toBe(1);
    expect(END_REASON.BIRD).toBe(2);
    expect(END_REASON.GHOST).toBe(3);
  });

  it('all values are unique', () => {
    const values = Object.values(END_REASON);
    expect(new Set(values).size).toBe(values.length);
  });
});

describe('endReasonLabel', () => {
  it.each([
    [END_REASON.GROUND, '气球落地'],
    [END_REASON.BIRD, '被鸟撞到'],
    [END_REASON.GHOST, '被幽灵碰到'],
    [END_REASON.NONE, ''],
    [99, ''],
    [-1, ''],
    [100, ''],
  ])('maps code %i to %j', (code, expected) => {
    expect(endReasonLabel(code)).toBe(expected);
  });
});
