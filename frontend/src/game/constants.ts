import { COOLDOWN as SHARED_COOLDOWN, PHYSICS } from '../shared/constants.js';
import { MSG_TYPE, CLIENT_MSG, PHASE_CODE } from '../shared/protocol.js';

export { COOLDOWN, PHYSICS } from '../shared/constants.js';
export { MSG_TYPE, CLIENT_MSG, PHASE_CODE } from '../shared/protocol.js';

export const textEncoder: TextEncoder = new TextEncoder();
export const textDecoder: TextDecoder = new TextDecoder();

export const PALETTE_COLORS: readonly string[] = [
  '#e94560', '#0f3460', '#533483', '#00b4d8',
  '#06d6a0', '#ffd166', '#ef476f', '#118ab2',
  '#073b4c', '#f78c6b',
] as const;

export const MAX_RECONNECT_ATTEMPTS: number = 10;
export const BASE_RECONNECT_DELAY: number = 1000;
export const HEARTBEAT_INTERVAL_MS: number = 25000;
export const HEARTBEAT_TIMEOUT_MS: number = 60000;
export const MAX_SEEN_SEQS: number = 200;
export const MAX_PENDING_QUEUE: number = 50;
export const FIXED_STEP: number = 1000 / 60;
