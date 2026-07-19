import { describe, it, expect, beforeEach } from 'vitest';
import { dispatch, getState } from './state.js';

beforeEach(() => {
  dispatch({ type: 'RESET_ALL' });
});

describe('store', () => {
  // createDefaultState, SET_STATE, RESET_ROUND, RESET_ALL, ADD_RIPPLE, SET_END_REASON
  // invariants are covered by reducer.property.test.ts. This file keeps one
  // integration test verifying multi-action dispatch sequences.
  it('getState returns updated state after multiple dispatches', () => {
    dispatch({ type: 'SET_STATE', partial: { phase: 'playing', score: 50 } });
    dispatch({ type: 'ADD_RIPPLE', ripple: { playerIndex: 1, x: 0.3, y: 0.7, time: Date.now() } });
    expect(getState().phase).toBe('playing');
    expect(getState().score).toBe(50);
    expect(getState().ripples).toHaveLength(1);
  });
});
