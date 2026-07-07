import type { GamePhase } from '../shared/game/types.js';

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

interface ClientRipple {
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
}

export type EntryStep = 'connecting' | 'error' | 'nickname' | 'waiting' | 'handoff';

export const state: ClientState = {
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
};
