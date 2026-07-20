import type { EntryStep } from './entry_flow.js';
export type GamePhase = 'waiting' | 'countdown' | 'playing' | 'ended';

interface ClientBalloon {
  x: number;
  y: number;
  vx: number;
  vy: number;
}

interface ClientBird {
  x: number;
  y: number;
  active: boolean;
}

interface ClientGhost {
  x: number;
  y: number;
  active: boolean;
  repelTimer: number;
}

export interface ClientPlayer {
  playerIndex: number;
  nickname: string;
  palette: number;
  cooldownEndTime: number;
  scoreContribution: number;
}

export interface ClientRipple {
  playerIndex: number;
  x: number;
  y: number;
  time: number;
  rejected?: boolean;
  isOptimistic?: boolean;
}

interface ClientRestartVotes {
  yes: number;
  total: number;
  countdownMs: number;
  receivedAt?: number;
}

export interface ClientState {
  phase: GamePhase;
  balloon: ClientBalloon;
  bird: ClientBird;
  ghost: ClientGhost;
  players: ClientPlayer[];
  myCooldownEnd: number;
  score: number;
  ripples: ClientRipple[];
  lobbyCode: string;
  lastTapX: number | null;
  lastTapY: number | null;
  connectionError: string | null;
  wind: number;
  restartVotes: ClientRestartVotes;
  hasReceivedFirstSnapshot: boolean;
  explosionEffect: { x: number; y: number; startTime: number } | null;
  restartClicked: boolean;
  nicknameSubmitted: boolean;
  pendingNickname: string | null;
  countdownTimerInterval: ReturnType<typeof setInterval> | null;
  endReason: number | null;
  wasEverConnected: boolean;
  blockGameRender: boolean;
  entryStep: EntryStep;
  // RO-041: Migrated from module-level `let` to store dispatch.
  wsConnectInFlight: boolean;
  connectedLobbyCode: string | null;
  lobbyPublished: boolean;
  wsConnected: boolean;
}

export type GameAction =
  | { type: 'SET_STATE'; partial: Partial<ClientState> }
  | { type: 'ADD_RIPPLE'; ripple: ClientRipple }
  | { type: 'SET_END_REASON'; reason: number | null }
  | { type: 'RESET_ROUND' }
  | { type: 'RESET_ALL' };

export function createDefaultState(): ClientState {
  return {
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
    entryStep: 'connecting',
    // RO-041: Migrated from module-level `let` to store dispatch.
    wsConnectInFlight: false,
    connectedLobbyCode: null,
    lobbyPublished: false,
    wsConnected: false,
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
      return { ...state, ...createDefaultState() };
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
    endReason: null,
    score: 0,
    balloon: { x: 0.5, y: 0.95, vx: 0, vy: 0 },
    bird: { x: 0, y: 0, active: false },
    ghost: { x: 0, y: 0, active: false, repelTimer: 0 },
    wind: 0,
  };
}

const _state: ClientState = createDefaultState();

export function getState(): ClientState {
  return _state;
}

/** @deprecated Only for test use. Production code must use getState(). */
export { _state as state };

export function dispatch(action: GameAction): void {
  const state = _state;
  const newState = gameReducer(state, action);
  if (newState !== state) {
    Object.assign(state, newState);
  }
}
