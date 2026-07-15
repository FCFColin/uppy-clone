import type { ClientState } from './state_types.js';
import { getState as getModuleState } from './state_types.js';
import { gameReducer, type GameAction } from './reducer.js';

type Subscriber = () => void;

const subscribers: Set<Subscriber> = new Set();

export function subscribe(fn: Subscriber): () => void {
  subscribers.add(fn);
  return () => { subscribers.delete(fn); };
}

export function dispatch(action: GameAction): void {
  const state = getModuleState();
  const newState = gameReducer(state, action);
  if (newState !== state) {
    Object.assign(state, newState);
    subscribers.forEach(fn => fn());
  }
}

export function getState(): ClientState {
  return getModuleState();
}
