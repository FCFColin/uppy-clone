import type { GamePhase } from '../shared/types.js';
import { MAX_SEEN_SEQS } from './constants.js';

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

interface ClientPlayer {
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
  optimistic?: boolean;
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
  countdownTimerInterval: ReturnType<typeof setInterval> | null;
}

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
  countdownTimerInterval: null,
};

interface InterpPoint {
  x: number;
  y: number;
}

interface InterpGhostPoint {
  x: number;
  y: number;
  active: boolean;
}

let prevSnapshotTime: number = 0;
let prevBalloon: InterpPoint | null = null;
let currSnapshotTime: number = 0;
let currBalloon: InterpPoint = { x: 0.5, y: 0.95 };
let prevGhost: InterpGhostPoint | null = null;
let currGhost: InterpGhostPoint = { x: 0.5, y: 0.5, active: false };

export function updateInterpolation(): void {
  const newBalloon: InterpPoint = { x: state.balloon.x, y: state.balloon.y };
  const newGhost: InterpGhostPoint = { x: state.ghost.x, y: state.ghost.y, active: state.ghost.active };

  if (prevBalloon === null) {
    prevBalloon = { x: newBalloon.x, y: newBalloon.y };
    currBalloon = { x: newBalloon.x, y: newBalloon.y };
    prevGhost = { x: newGhost.x, y: newGhost.y, active: newGhost.active };
    currGhost = { x: newGhost.x, y: newGhost.y, active: newGhost.active };
    prevSnapshotTime = Date.now() - 66;
    currSnapshotTime = Date.now();
    return;
  }

  const TELEPORT_THRESHOLD: number = 0.05;
  const balloonDx: number = Math.abs(newBalloon.x - currBalloon.x);
  const balloonDy: number = Math.abs(newBalloon.y - currBalloon.y);
  const ghostDx: number = Math.abs(newGhost.x - currGhost.x);
  const ghostDy: number = Math.abs(newGhost.y - currGhost.y);

  if (balloonDx > TELEPORT_THRESHOLD || balloonDy > TELEPORT_THRESHOLD) {
    console.log(`[interp] BALLOON teleport: dx=${balloonDx.toFixed(4)} dy=${balloonDy.toFixed(4)} — snapping, no interpolation`);
    prevBalloon = { x: newBalloon.x, y: newBalloon.y };
  } else {
    prevBalloon = { x: currBalloon.x, y: currBalloon.y };
  }

  if (ghostDx > TELEPORT_THRESHOLD || ghostDy > TELEPORT_THRESHOLD) {
    console.log(`[interp] GHOST teleport: dx=${ghostDx.toFixed(4)} dy=${ghostDy.toFixed(4)} — snapping, no interpolation`);
    prevGhost = { x: newGhost.x, y: newGhost.y, active: newGhost.active };
  } else {
    prevGhost = { x: currGhost.x, y: currGhost.y, active: currGhost.active };
  }

  prevSnapshotTime = currSnapshotTime;
  currBalloon = newBalloon;
  currGhost = newGhost;
  currSnapshotTime = Date.now();
}

export function resetInterpolation(): void {
  prevBalloon = null;
  prevGhost = null;
  currBalloon = { x: 0.5, y: 0.95 };
  currGhost = { x: 0.5, y: 0.5, active: false };
  currSnapshotTime = 0;
  prevSnapshotTime = 0;
  console.log(`[interp] interpolation state CLEARED (game state mutation)`);
}

export function getInterpolatedBalloon(): InterpPoint {
  if (prevBalloon === null) return currBalloon;
  const now: number = Date.now();
  const snapshotInterval: number = currSnapshotTime - prevSnapshotTime;
  if (snapshotInterval <= 0) return currBalloon;
  const elapsed: number = now - currSnapshotTime;
  const alpha: number = Math.min(1, elapsed / snapshotInterval);
  return {
    x: prevBalloon.x + (currBalloon.x - prevBalloon.x) * alpha,
    y: prevBalloon.y + (currBalloon.y - prevBalloon.y) * alpha,
  };
}

export function getInterpolatedGhost(): InterpGhostPoint | null {
  if (!currGhost.active) return null;
  if (prevGhost === null) return currGhost;
  const now: number = Date.now();
  const snapshotInterval: number = currSnapshotTime - prevSnapshotTime;
  if (snapshotInterval <= 0) return currGhost;
  const elapsed: number = now - currSnapshotTime;
  const alpha: number = Math.min(1, elapsed / snapshotInterval);
  return {
    x: prevGhost.x + (currGhost.x - prevGhost.x) * alpha,
    y: prevGhost.y + (currGhost.y - prevGhost.y) * alpha,
    active: true,
  };
}

export const seenSeqs: Set<number> = new Set();

export function isDuplicateSeq(seq: number): boolean {
  if (seenSeqs.has(seq)) return true;
  seenSeqs.add(seq);
  if (seenSeqs.size > MAX_SEEN_SEQS) {
    const toRemove: number = Math.floor(MAX_SEEN_SEQS / 2);
    let i: number = 0;
    for (const s of seenSeqs) {
      seenSeqs.delete(s);
      i++;
      if (i >= toRemove) break;
    }
  }
  return false;
}

export const pendingQueue: ArrayBuffer[] = [];

export function resetClientState(): void {
  state.hasReceivedFirstSnapshot = false;
  seenSeqs.clear();
  state.score = 0;
  state.myCooldownEnd = 0;
  state.ripples = [];
  state.lastTapX = null;
  state.lastTapY = null;
  state.balloon = { x: 0.5, y: 0.5, vx: 0, vy: 0 };
  state.bird = { x: 0, y: 0, active: false };
  state.ghost = { x: 0, y: 0, active: false, repelTimer: 0 };
  state.wind = 0;
  state.explosionEffect = null;
  pendingQueue.length = 0;
  state.restartClicked = false;
  state.restartVotes = { yes: 0, total: 0, countdownMs: 0 };
}

export function getInterpState(): {
  get prevBalloon(): InterpPoint | null;
  get currBalloon(): InterpPoint;
  get prevGhost(): InterpGhostPoint | null;
  get currGhost(): InterpGhostPoint;
} {
  return {
    get prevBalloon() { return prevBalloon; },
    get currBalloon() { return currBalloon; },
    get prevGhost() { return prevGhost; },
    get currGhost() { return currGhost; },
  };
}
