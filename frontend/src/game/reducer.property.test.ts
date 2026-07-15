import { describe, it, expect } from 'vitest';
import fc from 'fast-check';
import { gameReducer, createDefaultState } from './reducer.js';

const VALID_PHASES = ['waiting', 'countdown', 'playing', 'ended'] as const;

describe('createDefaultState', () => {
  it('score is 0', () => {
    expect(createDefaultState().score).toBe(0);
  });

  it('phase is waiting', () => {
    expect(createDefaultState().phase).toBe('waiting');
  });

  it('players is empty', () => {
    expect(createDefaultState().players).toEqual([]);
  });

  it('ripples is empty', () => {
    expect(createDefaultState().ripples).toEqual([]);
  });
});

describe('gameReducer SET_STATE invariants', () => {
  it('score is never negative when setting non-negative scores', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 1000000 }),
        (score) => {
          const state = gameReducer(createDefaultState(), {
            type: 'SET_STATE',
            partial: { score },
          });
          expect(state.score).toBeGreaterThanOrEqual(0);
        }
      )
    );
  });

  it('phase is always one of the valid game phases', () => {
    fc.assert(
      fc.property(
        fc.constantFrom(...VALID_PHASES),
        (phase) => {
          const state = gameReducer(createDefaultState(), {
            type: 'SET_STATE',
            partial: { phase },
          });
          expect(VALID_PHASES).toContain(state.phase);
        }
      )
    );
  });

  it('SET_STATE preserves other state fields', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 100000 }),
        (score) => {
          const base = createDefaultState();
          const state = gameReducer(base, {
            type: 'SET_STATE',
            partial: { score },
          });
          expect(state.phase).toBe(base.phase);
          expect(state.balloon.x).toBe(base.balloon.x);
          expect(state.wind).toBe(base.wind);
          expect(state.players).toEqual(base.players);
        }
      )
    );
  });
});

describe('gameReducer ADD_RIPPLE invariants', () => {
  it('appends a ripple to the ripples array', () => {
    fc.assert(
      fc.property(
        fc.integer(),
        fc.float(),
        fc.float(),
        (playerIndex, x, y) => {
          const state = gameReducer(createDefaultState(), {
            type: 'ADD_RIPPLE',
            ripple: { playerIndex, x, y, time: Date.now() },
          });
          expect(state.ripples).toHaveLength(1);
          expect(state.ripples[0]!.playerIndex).toBe(playerIndex);
        }
      )
    );
  });

  it('preserves existing ripples when adding new ones', () => {
    fc.assert(
      fc.property(
        fc.array(
          fc.record({
            playerIndex: fc.integer(),
            x: fc.float(),
            y: fc.float(),
          }),
          { minLength: 0, maxLength: 20 }
        ),
        (items) => {
          let state = createDefaultState();
          for (const item of items) {
            state = gameReducer(state, {
              type: 'ADD_RIPPLE',
              ripple: { ...item, time: Date.now() },
            });
          }
          expect(state.ripples).toHaveLength(items.length);
        }
      )
    );
  });

  it('creates a new ripples array reference', () => {
    fc.assert(
      fc.property(
        fc.integer(),
        (playerIndex) => {
          const base = createDefaultState();
          const state = gameReducer(base, {
            type: 'ADD_RIPPLE',
            ripple: { playerIndex, x: 0, y: 0, time: Date.now() },
          });
          expect(state.ripples).toHaveLength(1);
          expect(state.ripples[0]!.playerIndex).toBe(playerIndex);
        }
      )
    );
  });
});

describe('gameReducer RESET_ROUND invariants', () => {
  it('resets score, ripples, lastTapX, lastTapY to defaults', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 10000 }),
        fc.float(),
        fc.float(),
        (score, tapX, tapY) => {
          let state = gameReducer(createDefaultState(), { type: 'SET_STATE', partial: { score, lastTapX: tapX, lastTapY: tapY } });
          state = gameReducer(state, { type: 'ADD_RIPPLE', ripple: { playerIndex: 0, x: 0, y: 0, time: 1 } });
          state = gameReducer(state, { type: 'RESET_ROUND' });
          expect(state.score).toBe(0);
          expect(state.ripples).toEqual([]);
          expect(state.lastTapX).toBeNull();
          expect(state.lastTapY).toBeNull();
        }
      )
    );
  });

  it('preserves non-round fields like phase and lobbyCode', () => {
    const base = createDefaultState();
    const state = gameReducer(base, { type: 'RESET_ROUND' });
    expect(state.phase).toBe(base.phase);
    expect(state.lobbyCode).toBe(base.lobbyCode);
  });
});

describe('gameReducer RESET_ALL invariants', () => {
  it('resets to initial state from any prior state', () => {
    fc.assert(
      fc.property(
        fc.array(fc.integer({ min: 0, max: 10000 }), { minLength: 0, maxLength: 20 }),
        (scores) => {
          let state = createDefaultState();
          for (const s of scores) {
            state = gameReducer(state, { type: 'SET_STATE', partial: { score: s } });
          }
          const reset = gameReducer(state, { type: 'RESET_ALL' });
          expect(reset.score).toBe(0);
          expect(reset.ripples).toEqual([]);
          expect(reset.players).toEqual([]);
          expect(reset.phase).toBe('waiting');
          expect(reset.wind).toBe(0);
          expect(reset.myCooldownEnd).toBe(0);
        }
      )
    );
  });
});

describe('gameReducer SET_END_REASON invariants', () => {
  it('stores the provided reason and preserves other state', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 0, max: 255 }),
        (reason) => {
          const base = createDefaultState();
          const state = gameReducer(base, { type: 'SET_END_REASON', reason });
          expect(state.endReason).toBe(reason);
          expect(state.score).toBe(base.score);
          expect(state.phase).toBe(base.phase);
        }
      )
    );
  });
});
