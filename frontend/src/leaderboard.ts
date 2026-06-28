export {};

type Scope = 'global' | 'weekly';

interface Entry {
  rank: number;
  score: number;
  lobbyCode: string;
  endedAt: number;
}

const listEl = document.getElementById('leaderboard-list')!;
const emptyEl = document.getElementById('leaderboard-empty')!;
const tabGlobal = document.getElementById('tab-global')!;
const tabWeekly = document.getElementById('tab-weekly')!;

let scope: Scope = 'global';

const EMPTY_TEXT = '暂无记录，快去开一局吧！';

function appendLeaderboardItem(parent: HTMLElement, e: Entry): void {
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

async function load(): Promise<void> {
  listEl.textContent = '';
  emptyEl.textContent = EMPTY_TEXT;
  try {
    const res = await fetch(`/api/v1/leaderboard?scope=${scope}&limit=50`);
    if (!res.ok) throw new Error(`load failed (${res.status})`);
    const data: { entries: Entry[] } = await res.json();
    if (!data.entries?.length) {
      emptyEl.classList.remove('hidden');
      return;
    }
    emptyEl.classList.add('hidden');
    for (const e of data.entries) {
      appendLeaderboardItem(listEl, e);
    }
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
if (backBtn && gameUrl) {
  backBtn.hidden = false;
  backBtn.addEventListener('click', () => {
    window.location.href = gameUrl;
  });
}

void load();
