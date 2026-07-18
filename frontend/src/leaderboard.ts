export {};

import { fetchLeaderboard, renderLeaderboardEntries, type Scope } from './shared/ui/leaderboard_utils.js';

const listEl = document.getElementById('leaderboard-list')!;
const emptyEl = document.getElementById('leaderboard-empty')!;
const tabGlobal = document.getElementById('tab-global')!;
const tabWeekly = document.getElementById('tab-weekly')!;

let scope: Scope = 'global';

const EMPTY_TEXT = '暂无记录，快去开一局吧！';

async function load(): Promise<void> {
  listEl.textContent = '';
  emptyEl.textContent = EMPTY_TEXT;
  try {
    const entries = await fetchLeaderboard(scope, 50);
    if (!entries.length) {
      emptyEl.classList.remove('hidden');
      return;
    }
    emptyEl.classList.add('hidden');
    renderLeaderboardEntries(listEl, entries);
  } catch {
    emptyEl.textContent = '加载失败，请确认后端已启动并刷新页面';
    emptyEl.classList.remove('hidden');
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

// Show "返回游戏" button when a game URL was saved (opened from game page).
const backBtn = document.getElementById('back-to-game-btn');
const gameUrl = localStorage.getItem('uppy-game-url');
function isSafeLocalUrl(url: string): boolean {
  if (url.startsWith('//')) return false;
  if (url.startsWith('/')) return true;
  try {
    return new URL(url, window.location.origin).origin === window.location.origin;
  } catch {
    return false;
  }
}
if (backBtn && gameUrl && isSafeLocalUrl(gameUrl)) {
  backBtn.hidden = false;
  backBtn.addEventListener('click', () => {
    window.location.href = gameUrl;
  });
}

void load();
