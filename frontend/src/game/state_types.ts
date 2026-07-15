import type { EntryStep } from './entry_flow_ui.js';
import type { GamePhase } from '../shared/game/types.js';
import { createDefaultState } from './reducer.js';

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

const _state: ClientState = createDefaultState();

export function getState(): ClientState {
  return _state;
}

/** @internal Update the module-level state reference. Used by store.dispatch(). */
export function setState(newState: ClientState): void {
  // Assign each property to preserve the reference for consumers that hold
  // the old object (including the deprecated `state` export used in tests).
  Object.assign(_state, newState);
}

/** @deprecated Only for test use. Production code must use getState(). */
export { _state as state };

export function resetStateForTest(): void {
  Object.assign(_state, createDefaultState());
}
