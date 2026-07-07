import type { ClientState } from './state_types.js';
import { state } from './state_types.js';
import { gameReducer, type GameAction } from './reducer.js';

export function dispatch(action: GameAction): void {
  const newState = gameReducer(state, action);
  if (newState !== state) {
    Object.assign(state, newState);
  }
}

export function getState(): ClientState {
  return state;
}


