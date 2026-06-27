import type { GamePhase } from '../shared/types.js';
import { PHASE_CODE } from '../shared/protocol.js';
import { COOLDOWN, CLIENT_MSG, textEncoder } from './constants.js';

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
    Math.round(COOLDOWN.BASE_MS + COOLDOWN.LOG_COEFFICIENT * Math.log2(Math.max(1, playerCount)))
  );
}

export function encodeSetNickname(nickname: string): ArrayBuffer {
  const truncated = truncateNickname(nickname);
  const nickBytes: Uint8Array = textEncoder.encode(truncated);
  const buf: ArrayBuffer = new ArrayBuffer(1 + 1 + nickBytes.length);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.SET_NICKNAME);
  dv.setUint8(1, nickBytes.length);
  new Uint8Array(buf, 2).set(nickBytes);
  return buf;
}
