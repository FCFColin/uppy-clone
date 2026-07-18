import { apiFetch } from '../network/api_fetch.js';

export interface LeaderboardEntry {
  rank: number;
  score: number;
  lobbyCode: string;
}

export type Scope = 'global' | 'weekly';

export function renderLeaderboardEntry(parent: HTMLElement, e: LeaderboardEntry): void {
  const li = document.createElement('li');
  li.className = 'leaderboard-item';

  const rank = document.createElement('span');
  rank.className = 'lb-rank';
  rank.textContent = `#${e.rank}`;

  const score = document.createElement('span');
  score.className = 'lb-score';
  score.textContent = String(e.score);

  const code = document.createElement('span');
  code.className = 'lb-code';
  code.textContent = e.lobbyCode;

  li.append(rank, score, code);
  parent.appendChild(li);
}

export function renderLeaderboardEntries(parent: HTMLElement, entries: LeaderboardEntry[]): void {
  parent.textContent = '';
  for (const e of entries) {
    renderLeaderboardEntry(parent, e);
  }
}

export async function fetchLeaderboard(scope: Scope, limit: number): Promise<LeaderboardEntry[]> {
  const res = await apiFetch(`/api/v1/leaderboard?scope=${scope}&limit=${limit}`, { autoRefresh: false });
  if (!res.ok) throw new Error(`load failed (${res.status})`);
  const data: { entries: LeaderboardEntry[] } = await res.json();
  return data.entries ?? [];
}
