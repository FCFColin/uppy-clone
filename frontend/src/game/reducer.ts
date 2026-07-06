import type { ClientState } from './state_types.js';
import type { GamePhase } from '../shared/game/types.js';

interface ClientRipple {
  playerIndex: number;
  x: number;
  y: number;
  time: number;
  rejected?: boolean;
  isOptimistic?: boolean;
}

export type GameAction =
  | { type: 'SET_STATE'; partial: Partial<ClientState> }
  | { type: 'ADD_RIPPLE'; ripple: ClientRipple }
  | { type: 'SET_END_REASON'; reason: number }
  | { type: 'RESET_ROUND' }
  | { type: 'RESET_ALL' };

const INITIAL_STATE: ClientState = {
  phase: 'waiting',
  balloon: { x: 0.5, y: 0.5, vx: 0, vy: 0 },
  bird: { x: 0, y: 0, active: false },
  ghost: { x: 0, y: 0, active: false, repelTimer: 0 },
  players: [],
  myCooldownEnd: 0,
  score: 0,
  ripples: [],
  lobbyCode: '',
  lastTapX: null,
  lastTapY: null,
  connectionError: null,
  wind: 0,
  restartVotes: { yes: 0, total: 0, countdownMs: 0 },
  hasReceivedFirstSnapshot: false,
  explosionEffect: null,
  restartClicked: false,
  nicknameSubmitted: false,
  pendingNickname: null,
  countdownTimerInterval: null,
  endReason: null,
  wasEverConnected: false,
  blockGameRender: false,
};

export function createInitialState(): ClientState {
  return {
    ...INITIAL_STATE,
    ripples: [],
    players: [],
    balloon: { ...INITIAL_STATE.balloon },
    bird: { ...INITIAL_STATE.bird },
    ghost: { ...INITIAL_STATE.ghost },
    restartVotes: { ...INITIAL_STATE.restartVotes },
  };
}

export function gameReducer(state: ClientState, action: GameAction): ClientState {
  switch (action.type) {
    case 'SET_STATE':
      return { ...state, ...action.partial };
    case 'ADD_RIPPLE':
      return { ...state, ripples: [...state.ripples, action.ripple] };
    case 'SET_END_REASON':
      return { ...state, endReason: action.reason };
    case 'RESET_ROUND':
      return resetRound(state);
    case 'RESET_ALL': {
      return { ...state, ...createInitialState() };
    }
    default:
      return state;
  }
}

function resetRound(state: ClientState): ClientState {
  return {
    ...state,
    ripples: [],
    explosionEffect: null,
    myCooldownEnd: 0,
    lastTapX: null,
    lastTapY: null,
    restartClicked: false,
    restartVotes: { yes: 0, total: 0, countdownMs: 0 },
    score: 0,
    balloon: { x: 0.5, y: 0.95, vx: 0, vy: 0 },
    bird: { x: 0, y: 0, active: false },
    ghost: { x: 0, y: 0, active: false, repelTimer: 0 },
    wind: 0,
  };
}
