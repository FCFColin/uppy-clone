import { describe, it, expect } from 'vitest';
import { END_REASON } from '../shared/game/constants.js';
import { endReasonLabel } from './constants.js';

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
