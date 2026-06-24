import type { GamePhase } from '../shared/types.js';
import { COOLDOWN, CLIENT_MSG, textEncoder } from './constants.js';

export function codeToPhase(code: number): GamePhase {
  switch (code) {
    case 0: return 'waiting';
    case 1: return 'playing';
    case 2: return 'ended';
    case 3: return 'countdown';
    default: return 'waiting';
  }
}

export function calculateCooldown(playerCount: number): number {
  return Math.min(
    COOLDOWN.MAX_MS,
    Math.round(COOLDOWN.BASE_MS + COOLDOWN.LOG_COEFFICIENT * Math.log2(Math.max(1, playerCount)))
  );
}

export function encodeSetNickname(nickname: string): ArrayBuffer {
  const nickBytes: Uint8Array = textEncoder.encode(nickname);
  const buf: ArrayBuffer = new ArrayBuffer(1 + 1 + nickBytes.length);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.SET_NICKNAME);
  dv.setUint8(1, nickBytes.length);
  new Uint8Array(buf, 2).set(nickBytes);
  return buf;
}
