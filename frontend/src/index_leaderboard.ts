import {
  fetchLeaderboard,
  type Scope,
  type LeaderboardEntry,
} from './shared/ui/ui.js';
import { t } from './i18n/t.js';

type Entry = LeaderboardEntry;

let scope: Scope = 'global';

const PREVIEW_LIMIT = 5;

const AVATAR_COLORS: readonly string[] = [
  '#e94560', '#3b82f6', '#22c55e', '#a855f7',
  '#f59e0b', '#06d6a0', '#ec4899', '#6366f1',
  '#ef4444', '#10b981', '#8b5cf6', '#f97316',
];

function setActiveTab(next: Scope): void {
  scope = next;
  document.getElementById('lb-preview-global')?.classList.toggle('active', next === 'global');
  document.getElementById('lb-preview-weekly')?.classList.toggle('active', next === 'weekly');
}

function avatarFor(name: string, colorIndex: number): HTMLElement {
  const span = document.createElement('span');
  span.className = 'avatar';
  const ch = name.trim().charAt(0) || '?';
  span.textContent = ch.toUpperCase();
  span.style.background = AVATAR_COLORS[colorIndex % AVATAR_COLORS.length] ?? '#64748b';
  return span;
}

function renderEntry(parent: HTMLElement, e: Entry): void {
  const li = document.createElement('li');
  li.className = 'lb-item';
  if (e.rank === 1) li.classList.add('lb-rank-1');
  else if (e.rank === 2) li.classList.add('lb-rank-2');
  else if (e.rank === 3) li.classList.add('lb-rank-3');

  const rank = document.createElement('span');
  rank.className = 'lb-rank';
  rank.textContent = String(e.rank);

  const avatar = avatarFor(e.name, e.rank);

  const name = document.createElement('span');
  name.className = 'lb-name';
  name.textContent = e.name;

  const score = document.createElement('span');
  score.className = 'lb-score';
  score.textContent = e.score.toLocaleString();

  li.append(rank, avatar, name, score);
  parent.appendChild(li);
}

async function loadPreview(): Promise<void> {
  const listEl = document.getElementById('leaderboard-preview');
  const emptyEl = document.getElementById('leaderboard-preview-empty');
  const errorEl = document.getElementById('leaderboard-preview-error');
  if (!listEl) return;

  emptyEl?.classList.add('hidden');
  errorEl?.classList.add('hidden');
  listEl.textContent = '';

  try {
    const entries = await fetchLeaderboard(scope, PREVIEW_LIMIT);
    if (!entries.length) {
      emptyEl?.classList.remove('hidden');
      return;
    }
    for (const e of entries) {
      renderEntry(listEl, e);
    }
  } catch {
    if (errorEl) {
      errorEl.textContent = t('leaderboard_page.load_failed_hint');
      errorEl.classList.remove('hidden');
    }
  }
}

export function initCollapsibleLeaderboard(): void {
  document.getElementById('lb-preview-global')?.addEventListener('click', () => {
    setActiveTab('global');
    void loadPreview();
  });
  document.getElementById('lb-preview-weekly')?.addEventListener('click', () => {
    setActiveTab('weekly');
    void loadPreview();
  });
  void loadPreview();
}
