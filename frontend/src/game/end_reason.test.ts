import { describe, it, expect } from 'vitest';
import { END_REASON, endReasonLabel } from './end_reason.js';

describe('endReasonLabel', () => {
  it('maps known end reason codes to labels', () => {
    expect(endReasonLabel(END_REASON.GROUND)).toBe('气球落地');
    expect(endReasonLabel(END_REASON.BIRD)).toBe('被鸟撞到');
    expect(endReasonLabel(END_REASON.GHOST)).toBe('被幽灵碰到');
  });

  it('returns empty string for unknown or none codes', () => {
    expect(endReasonLabel(END_REASON.NONE)).toBe('');
    expect(endReasonLabel(99)).toBe('');
  });
});
