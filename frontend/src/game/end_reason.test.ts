import { describe, it, expect } from 'vitest';
import { END_REASON, endReasonLabel } from './local_constants.js';

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
  it('maps known end reason codes to labels', () => {
    expect(endReasonLabel(END_REASON.GROUND)).toBe('气球落地');
    expect(endReasonLabel(END_REASON.BIRD)).toBe('被鸟撞到');
    expect(endReasonLabel(END_REASON.GHOST)).toBe('被幽灵碰到');
  });

  it('returns empty string for NONE code', () => {
    expect(endReasonLabel(END_REASON.NONE)).toBe('');
  });

  it('returns empty string for unknown codes', () => {
    expect(endReasonLabel(99)).toBe('');
    expect(endReasonLabel(-1)).toBe('');
    expect(endReasonLabel(100)).toBe('');
  });
});
