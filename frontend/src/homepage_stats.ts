import { apiFetch } from './shared/network/api_fetch.js';

interface PublicStats {
  onlinePlayers: number;
  gamesToday: number;
  bestScore: number;
  activeRooms: number;
}

const POLL_INTERVAL_MS = 30_000;
const PLACEHOLDER = '--';

const STAT_FIELDS: readonly { id: string; key: keyof PublicStats; format: (n: number) => string }[] = [
  { id: 'stat-online-value', key: 'onlinePlayers', format: (n) => n.toLocaleString() },
  { id: 'stat-games-value', key: 'gamesToday', format: (n) => n.toLocaleString() },
  { id: 'stat-best-value', key: 'bestScore', format: (n) => n.toLocaleString() },
];

function setText(id: string, text: string): void {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
}

function applyStats(stats: PublicStats): void {
  for (const f of STAT_FIELDS) {
    setText(f.id, f.format(stats[f.key]));
  }
  setText('join-count', stats.activeRooms > 0 ? `+${stats.activeRooms}` : PLACEHOLDER);
}

function applyPlaceholder(): void {
  for (const f of STAT_FIELDS) {
    setText(f.id, PLACEHOLDER);
  }
  setText('join-count', PLACEHOLDER);
}

async function fetchOnce(): Promise<void> {
  try {
    const res = await apiFetch('/api/v1/stats/public', { autoRefresh: false });
    if (!res.ok) {
      applyPlaceholder();
      return;
    }
    const data: PublicStats = await res.json();
    applyStats(data);
  } catch {
    applyPlaceholder();
  }
}

let timer: ReturnType<typeof setInterval> | null = null;

function start(): void {
  if (timer !== null) return;
  void fetchOnce();
  timer = setInterval(() => {
    if (document.hidden) return;
    void fetchOnce();
  }, POLL_INTERVAL_MS);
}

function stop(): void {
  if (timer !== null) {
    clearInterval(timer);
    timer = null;
  }
}

export function initHomepageStats(): void {
  start();
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      stop();
    } else {
      start();
    }
  });
}
