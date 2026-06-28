import type { GamePhase } from '../shared/types.js';
import { textDecoder } from './constants.js';
import { codeToPhase } from './message_codec.js';

export interface DecodedPlayer {
  playerIndex: number;
  cooldownEndTime: number;
  palette: number;
  scoreContribution: number;
  nickname: string;
}

export interface DecodedSnapshot {
  timestamp: number;
  score: number;
  phase: GamePhase;
  balloon: { x: number; y: number; vx: number; vy: number };
  bird: { active: boolean; x: number; y: number };
  ghost: { active: boolean; x: number; y: number; repelTimer: number };
  players: DecodedPlayer[];
  ripples: Array<{ playerIndex: number; x: number; y: number; time: number }>;
  wind?: number;
  playerCount: number;
}

export function decodeSnapshot(view: DataView): DecodedSnapshot | null {
  if (view.byteLength < 37) {
    return null;
  }

  let o = 1;
  const timestamp = view.getUint32(o, true); o += 4;
  const score = view.getUint32(o, true); o += 4;
  const phaseCode = view.getUint8(o); o += 1;
  const phase = codeToPhase(phaseCode);

  const balloon = {
    x: view.getFloat32(o, true), y: view.getFloat32(o + 4, true),
    vy: view.getFloat32(o + 8, true), vx: view.getFloat32(o + 12, true),
  };
  o += 16;

  const birdActive = view.getUint8(o) === 1; o += 1;
  const bird = { active: birdActive, x: 0, y: 0 };
  if (birdActive) {
    bird.x = view.getFloat32(o, true); o += 4;
    bird.y = view.getFloat32(o, true); o += 4;
  }

  const ghost = {
    active: view.getUint8(o) === 1,
    x: view.getFloat32(o + 1, true),
    y: view.getFloat32(o + 5, true),
    repelTimer: view.getUint16(o + 9, true),
  };
  o += 11;

  const playerCount = view.getUint8(o); o += 1;
  const players: DecodedPlayer[] = [];
  const now = Date.now();
  for (let i = 0; i < playerCount; i++) {
    const playerIndex = view.getUint16(o, true); o += 2;
    const cooldownRemainingMs = view.getUint32(o, true); o += 4;
    const palette = view.getUint32(o, true); o += 4;
    const scoreContribution = view.getUint32(o, true); o += 4;
    const nickLen = view.getUint8(o); o += 1;
    const nickname = textDecoder.decode(new Uint8Array(view.buffer, view.byteOffset + o, nickLen));
    o += nickLen;
    players.push({ playerIndex, cooldownEndTime: now + cooldownRemainingMs, palette, scoreContribution, nickname });
  }

  const ripples: DecodedSnapshot['ripples'] = [];
  if (o < view.byteLength) {
    const rippleCount = view.getUint8(o); o += 1;
    for (let i = 0; i < rippleCount; i++) {
      const pIdx = view.getUint16(o, true); o += 2;
      const rx = view.getFloat32(o, true); o += 4;
      const ry = view.getFloat32(o, true); o += 4;
      ripples.push({ playerIndex: pIdx, x: rx, y: ry, time: Date.now() });
    }
  }

  let wind: number | undefined;
  if (o < view.byteLength) {
    wind = view.getFloat32(o, true);
  }

  return { timestamp, score, phase, balloon, bird, ghost, players, ripples, wind, playerCount };
}

export interface SnapshotApplyTarget {
  score: number;
  balloon: { x: number; y: number; vx: number; vy: number };
  bird: { active: boolean; x: number; y: number };
  ghost: { active: boolean; x: number; y: number; repelTimer: number };
  players: DecodedPlayer[];
}

export function applySnapshot(decoded: DecodedSnapshot, target: SnapshotApplyTarget): void {
  target.score = decoded.score;
  target.balloon.x = decoded.balloon.x;
  target.balloon.y = decoded.balloon.y;
  target.balloon.vx = decoded.balloon.vx;
  target.balloon.vy = decoded.balloon.vy;
  target.bird.active = decoded.bird.active;
  target.bird.x = decoded.bird.x;
  target.bird.y = decoded.bird.y;
  target.ghost.active = decoded.ghost.active;
  target.ghost.x = decoded.ghost.x;
  target.ghost.y = decoded.ghost.y;
  target.ghost.repelTimer = decoded.ghost.repelTimer;
  target.players = decoded.players;
}
