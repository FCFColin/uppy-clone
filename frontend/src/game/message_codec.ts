import type { GamePhase } from './state.js';
import { PHASE_CODE } from '../shared/game/constants.js';
import { COOLDOWN } from '../shared/game/constants.js';
import { CLIENT_MSG } from '../shared/game/constants.js';

const _textEncoder = new TextEncoder();
const _textDecoder = new TextDecoder();

const phaseByCode: Record<number, GamePhase> = {
  [PHASE_CODE.WAITING]: 'waiting',
  [PHASE_CODE.PLAYING]: 'playing',
  [PHASE_CODE.ENDED]: 'ended',
  [PHASE_CODE.COUNTDOWN]: 'countdown',
};

const MAX_NICKNAME_RUNES = 12;

export function truncateNickname(nickname: string): string {
  const runes = [...nickname];
  if (runes.length <= MAX_NICKNAME_RUNES) return nickname;
  return runes.slice(0, MAX_NICKNAME_RUNES).join('');
}

export function codeToPhase(code: number): GamePhase {
  return phaseByCode[code] ?? 'waiting';
}

export function calculateCooldown(playerCount: number): number {
  return Math.min(
    COOLDOWN.MAX_MS,
    Math.round(COOLDOWN.BASE_MS + COOLDOWN.LOG_COEFFICIENT * Math.log2(Math.max(1, playerCount))),
  );
}

export function encodeSetNickname(nickname: string): ArrayBuffer {
  const truncated = truncateNickname(nickname);
  const nickBytes: Uint8Array = _textEncoder.encode(truncated);
  if (nickBytes.length > 255) {
    throw new Error('nickname too long for uint8 length field');
  }
  const buf: ArrayBuffer = new ArrayBuffer(1 + 1 + nickBytes.length);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.SET_NICKNAME);
  dv.setUint8(1, nickBytes.length);
  new Uint8Array(buf, 2).set(nickBytes);
  return buf;
}

// ── Snapshot decoding ────────────────────────────────────────────

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

function readBalloon(view: DataView, offset: number) {
  return {
    balloon: {
      x: view.getFloat32(offset, true),
      y: view.getFloat32(offset + 4, true),
      vx: view.getFloat32(offset + 8, true),
      vy: view.getFloat32(offset + 12, true),
    },
    bytesRead: 16,
  };
}

function readBird(view: DataView, offset: number) {
  if (view.byteLength < offset + 1) {
    return { bird: { active: false, x: 0, y: 0 }, bytesRead: 0 };
  }
  const active = view.getUint8(offset) === 1;
  if (!active || view.byteLength < offset + 9) {
    return { bird: { active, x: 0, y: 0 }, bytesRead: 1 };
  }
  return {
    bird: { active, x: view.getFloat32(offset + 1, true), y: view.getFloat32(offset + 5, true) },
    bytesRead: 9,
  };
}

function readGhost(view: DataView, offset: number) {
  if (view.byteLength < offset + 1) {
    return { ghost: { active: false, x: 0, y: 0, repelTimer: 0 }, bytesRead: 0 };
  }
  const active = view.getUint8(offset) === 1;
  if (!active || view.byteLength < offset + 11) {
    return { ghost: { active, x: 0, y: 0, repelTimer: 0 }, bytesRead: 1 };
  }
  return {
    ghost: {
      active,
      x: view.getFloat32(offset + 1, true),
      y: view.getFloat32(offset + 5, true),
      repelTimer: view.getUint16(offset + 9, true),
    },
    bytesRead: 11,
  };
}

function clampedNickLength(view: DataView, offset: number): number {
  const maxLen = view.byteLength - offset - 1;
  return maxLen > 0 ? Math.min(view.getUint8(offset), maxLen) : 0;
}

function readPlayers(view: DataView, offset: number) {
  if (view.byteLength < offset + 1) {
    return { players: [], playerCount: 0, bytesRead: 0 };
  }
  const playerCount = view.getUint8(offset);
  let o = offset + 1;
  const players: DecodedPlayer[] = [];
  const now = Date.now();
  for (let i = 0; i < playerCount; i++) {
    if (view.byteLength < o + 15) break;
    const playerIndex = view.getUint16(o, true);
    o += 2;
    const cooldownRemainingMs = view.getUint32(o, true);
    o += 4;
    const palette = view.getUint32(o, true);
    o += 4;
    const scoreContribution = view.getUint32(o, true);
    o += 4;
    const nickLen = clampedNickLength(view, o);
    o += 1;
    let nickname = nickLen > 0 ? _textDecoder.decode(new Uint8Array(view.buffer, view.byteOffset + o, nickLen)) : '';
    nickname = truncateNickname(nickname);
    o += nickLen;
    players.push({ playerIndex, cooldownEndTime: now + cooldownRemainingMs, palette, scoreContribution, nickname });
  }
  return { players, playerCount, bytesRead: o - offset };
}

export function decodeSnapshot(view: DataView): DecodedSnapshot | null {
  if (view.byteLength < 37) {
    return null;
  }

  let o = 1;
  const timestamp = view.getUint32(o, true);
  o += 4;
  const score = view.getUint32(o, true);
  o += 4;
  const phaseCode = view.getUint8(o);
  o += 1;
  const phase = codeToPhase(phaseCode);

  const balloonResult = readBalloon(view, o);
  const balloon = balloonResult.balloon;
  o += balloonResult.bytesRead;

  const birdResult = readBird(view, o);
  const bird = birdResult.bird;
  o += birdResult.bytesRead;

  const ghostResult = readGhost(view, o);
  const ghost = ghostResult.ghost;
  o += ghostResult.bytesRead;

  const playersResult = readPlayers(view, o);
  const players = playersResult.players;
  const playerCount = playersResult.playerCount;
  o += playersResult.bytesRead;

  const ripples: DecodedSnapshot['ripples'] = [];
  if (o < view.byteLength) {
    const rippleCount = view.getUint8(o);
    o += 1;
    for (let i = 0; i < rippleCount; i++) {
      if (o + 10 > view.byteLength) break;
      const pIdx = view.getUint16(o, true);
      o += 2;
      const rx = view.getFloat32(o, true);
      o += 4;
      const ry = view.getFloat32(o, true);
      o += 4;
      ripples.push({ playerIndex: pIdx, x: rx, y: ry, time: Date.now() });
    }
  }

  let wind: number | undefined;
  if (o + 4 <= view.byteLength) {
    wind = view.getFloat32(o, true);
  }

  return { timestamp, score, phase, balloon, bird, ghost, players, ripples, wind, playerCount };
}

interface SnapshotApplyTarget {
  score: number;
  balloon: { x: number; y: number; vx: number; vy: number };
  bird: { active: boolean; x: number; y: number };
  ghost: { active: boolean; x: number; y: number; repelTimer: number };
  players: DecodedPlayer[];
}

export function applySnapshot(decoded: DecodedSnapshot, target?: SnapshotApplyTarget): SnapshotApplyTarget {
  const snapshot: SnapshotApplyTarget = {
    score: decoded.score,
    balloon: { ...decoded.balloon },
    bird: { ...decoded.bird },
    ghost: { ...decoded.ghost },
    players: decoded.players.map((p) => ({ ...p })),
  };
  if (target) {
    Object.assign(target, snapshot);
  }
  return snapshot;
}
