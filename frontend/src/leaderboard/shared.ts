import { fetchLeaderboard, type LeaderboardEntry, type Scope } from '../shared/ui/ui.js';
import { Trophy, Award } from '../icons.js';
import { t } from '../i18n/t.js';

export interface LeaderboardOptions {
  showBackToLobby: boolean;
  onClose?: () => void;
}

const AVATAR_COLORS: readonly string[] = [
  '#e94560', '#3b82f6', '#22c55e', '#a855f7',
  '#f59e0b', '#06d6a0', '#ec4899', '#6366f1',
  '#ef4444', '#10b981', '#8b5cf6', '#f97316',
];

function avatarFor(name: string, colorIndex: number): HTMLElement {
  const span = document.createElement('span');
  span.className = 'avatar';
  const ch = name.trim().charAt(0) || '?';
  span.textContent = ch.toUpperCase();
  span.style.background = AVATAR_COLORS[colorIndex % AVATAR_COLORS.length] ?? '#64748b';
  return span;
}

function renderEntry(e: LeaderboardEntry): HTMLElement {
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

  if (e.rank <= 3) {
    const badge = document.createElement('span');
    badge.className = 'lb-rank-badge';
    li.append(rank, avatar, name, score, badge);
  } else {
    li.append(rank, avatar, name, score);
  }

  return li;
}

function initRankBadges(listEl: HTMLElement): void {
  const items = listEl.querySelectorAll('.lb-item');
  items.forEach((item) => {
    const rankBadge = item.querySelector('.lb-rank-badge');
    if (!rankBadge) return;
    if (item.classList.contains('lb-rank-1')) {
      rankBadge.innerHTML = Trophy({ size: 32, color: '#fbbf24', strokeWidth: 2 });
    } else if (item.classList.contains('lb-rank-2')) {
      rankBadge.innerHTML = Award({ size: 32, color: '#e2e8f0', strokeWidth: 2 });
    } else if (item.classList.contains('lb-rank-3')) {
      rankBadge.innerHTML = Award({ size: 32, color: '#cd7f32', strokeWidth: 2 });
    }
  });
}

export async function renderLeaderboard(
  container: HTMLElement,
  options: LeaderboardOptions,
): Promise<void> {
  container.textContent = '';

  const tabs = document.createElement('div');
  tabs.className = 'lb-tabs glass-panel';

  const tabGlobal = document.createElement('button');
  tabGlobal.className = 'btn-tab active';
  tabGlobal.type = 'button';
  tabGlobal.textContent = t('index.total_tab');

  const tabWeekly = document.createElement('button');
  tabWeekly.className = 'btn-tab';
  tabWeekly.type = 'button';
  tabWeekly.textContent = t('index.weekly_tab');

  tabs.append(tabGlobal, tabWeekly);

  const tableCard = document.createElement('div');
  tableCard.className = 'lb-table-card glass-panel';

  const listEl = document.createElement('ol');
  listEl.className = 'lb-full-list';

  const emptyEl = document.createElement('p');
  emptyEl.className = 'hint hidden';
  emptyEl.textContent = t('leaderboard_page.no_records');

  const errorEl = document.createElement('p');
  errorEl.className = 'hint hidden';
  errorEl.textContent = t('leaderboard_page.load_failed');

  tableCard.append(listEl, emptyEl, errorEl);

  const footer = document.createElement('div');
  footer.className = 'lb-footer-actions';
  if (options.showBackToLobby) {
    const backToHall = document.createElement('a');
    backToHall.href = '/index.html';
    backToHall.className = 'btn-cta-primary lb-action-btn';
    backToHall.innerHTML =
      `<span class="btn-text">${t('common.back_lobby')}</span><span class="btn-icon-trailing"></span>`;
    footer.append(backToHall);
  }

  container.append(tabs, tableCard, footer);

  let scope: Scope = 'global';

  async function load(): Promise<void> {
    listEl.textContent = '';
    emptyEl.classList.add('hidden');
    errorEl.classList.add('hidden');
    try {
      const entries = await fetchLeaderboard(scope, 50);
      if (!entries.length) {
        emptyEl.classList.remove('hidden');
        return;
      }
      for (const e of entries) {
        listEl.appendChild(renderEntry(e));
      }
      initRankBadges(listEl);
    } catch {
      errorEl.textContent = t('leaderboard_page.load_failed_hint');
      errorEl.classList.remove('hidden');
    }
  }

  function setScope(next: Scope): void {
    scope = next;
    tabGlobal.classList.toggle('active', next === 'global');
    tabWeekly.classList.toggle('active', next === 'weekly');
    void load();
  }

  tabGlobal.addEventListener('click', () => setScope('global'));
  tabWeekly.addEventListener('click', () => setScope('weekly'));

  void load();
}
