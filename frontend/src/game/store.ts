import type { ClientState } from './state_types.js';
import { state as oldState } from './state_types.js';
import { gameReducer, type GameAction } from './reducer.js';

let _state: ClientState = oldState;

export function dispatch(action: GameAction): void {
  _state = gameReducer(_state, action);
}

export function getState(): ClientState {
  return _state;
}

export function select<T>(selector: (s: ClientState) => T): T {
  return selector(_state);
}
