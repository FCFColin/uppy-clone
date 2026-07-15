import { END_REASON } from '../shared/game/constants.js';

export const MAX_RECONNECT_ATTEMPTS = 10;
export const BASE_RECONNECT_DELAY = 1000;
export const HEARTBEAT_INTERVAL_MS = 25000;
export const HEARTBEAT_TIMEOUT_MS = 60000;
export const MAX_SEEN_SEQS = 200;
export const MAX_PENDING_QUEUE = 50;

const END_REASON_LABELS: Record<number, string> = {
  [END_REASON.GROUND]: '气球落地',
  [END_REASON.BIRD]: '被鸟撞到',
  [END_REASON.GHOST]: '被幽灵碰到',
};

export function endReasonLabel(code: number): string {
  return END_REASON_LABELS[code] ?? '';
}
