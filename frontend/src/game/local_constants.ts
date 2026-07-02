export const PALETTE_COLORS: readonly string[] = [
  '#e94560', '#0f3460', '#533483', '#00b4d8',
  '#06d6a0', '#ffd166', '#ef476f', '#118ab2',
  '#073b4c', '#f78c6b',
] as const;

export const MAX_RECONNECT_ATTEMPTS = 10;
export const BASE_RECONNECT_DELAY = 1000;
export const HEARTBEAT_INTERVAL_MS = 25000;
export const HEARTBEAT_TIMEOUT_MS = 60000;
export const MAX_SEEN_SEQS = 200;
export const MAX_PENDING_QUEUE = 50;
export const FIXED_STEP = 1000 / 60;
