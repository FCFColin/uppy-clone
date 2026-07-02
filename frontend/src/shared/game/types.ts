export type GamePhase = 'waiting' | 'countdown' | 'playing' | 'ended';

export interface BalloonState {
  x: number; y: number; vx: number; vy: number; score: number;
}

export interface BirdState {
  x: number; y: number; active: boolean; spawnTimer: number; speed: number;
}

export interface GhostState {
  x: number; y: number; active: boolean; spawnTimer: number;
  repelTimer: number; vx: number; vy: number;
}

export interface PlayerState {
  id: string; playerIndex: number; nickname: string; palette: number;
  cooldownEndTime: number; scoreContribution: number; tapsCount: number;
  disconnected: boolean; disconnectedAt?: number;
}

export interface Ripple {
  playerIndex: number; x: number; y: number;
}

export interface RestartVotes {
  yes: number; total: number; countdownMs: number;
}

export interface GameState {
  phase: GamePhase;
  balloon: BalloonState;
  bird: BirdState;
  ghost: GhostState;
  players: PlayerState[];
  myCooldownEnd: number;
  myPlayerIndex: number;
  ripples: Ripple[];
  lastTapX: number | null;
  lastTapY: number | null;
  wind: number;
  score: number;
  restartVotes: RestartVotes;
  tickCount: number;
}
