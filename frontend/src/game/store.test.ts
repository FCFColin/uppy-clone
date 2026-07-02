import { describe, it, expect, beforeEach } from 'vitest';
import { dispatch, getState, select } from './store.js';
import type { GameAction } from './reducer.js';

beforeEach(() => {
  dispatch({ type: 'RESET_ALL' });
});

describe('store', () => {
  it('getState returns initial state', () => {
    const s = getState();
    expect(s.phase).toBe('waiting');
    expect(s.ripples).toEqual([]);
    expect(s.score).toBe(0);
    expect(s.lobbyCode).toBe('');
  });

  it('dispatch SET_STATE updates a field', () => {
    dispatch({ type: 'SET_STATE', partial: { phase: 'playing' } });
    expect(getState().phase).toBe('playing');
  });

  it('select returns derived value', () => {
    dispatch({ type: 'SET_STATE', partial: { score: 100 } });
    const score = select(s => s.score);
    expect(score).toBe(100);
  });

  it('RESET_ROUND resets gameplay fields but keeps lobbyCode/pendingNickname', () => {
    dispatch({ type: 'SET_STATE', partial: {
      lobbyCode: 'ABC12',
      pendingNickname: 'Player1',
      score: 100,
      phase: 'playing',
      wind: 0.5,
    }});
    dispatch({ type: 'RESET_ROUND' });
    const s = getState();
    expect(s.lobbyCode).toBe('ABC12');
    expect(s.pendingNickname).toBe('Player1');
    expect(s.score).toBe(0);
    expect(s.phase).toBe('playing');
    expect(s.wind).toBe(0);
    expect(s.ripples).toEqual([]);
    expect(s.balloon.y).toBe(0.95);
  });

  it('RESET_ALL returns fresh initial state', () => {
    dispatch({ type: 'SET_STATE', partial: { score: 100, lobbyCode: 'ABC12' } });
    dispatch({ type: 'RESET_ALL' });
    const s = getState();
    expect(s.score).toBe(0);
    expect(s.lobbyCode).toBe('');
    expect(s.phase).toBe('waiting');
  });

  it('ADD_RIPPLE adds a ripple', () => {
    const ripple = { playerIndex: 0, x: 0.5, y: 0.5, time: Date.now() };
    dispatch({ type: 'ADD_RIPPLE', ripple });
    expect(getState().ripples).toHaveLength(1);
    expect(getState().ripples[0]!.playerIndex).toBe(0);
  });

  it('SET_END_REASON sets endReason', () => {
    dispatch({ type: 'SET_END_REASON', reason: 1 });
    expect(getState().endReason).toBe(1);
  });

  it('getState returns updated state after multiple dispatches', () => {
    dispatch({ type: 'SET_STATE', partial: { phase: 'playing', score: 50 } });
    dispatch({ type: 'ADD_RIPPLE', ripple: { playerIndex: 1, x: 0.3, y: 0.7, time: Date.now() } });
    expect(getState().phase).toBe('playing');
    expect(getState().score).toBe(50);
    expect(getState().ripples).toHaveLength(1);
  });
});
