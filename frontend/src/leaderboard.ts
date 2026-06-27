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

async function load(): Promise<void> {
  listEl.textContent = '';
  try {
    const res = await fetch(`/api/v1/leaderboard?scope=${scope}&limit=50`);
    if (!res.ok) throw new Error('load failed');
    const data: { entries: Entry[] } = await res.json();
    if (!data.entries?.length) {
      emptyEl.classList.remove('hidden');
      return;
    }
    emptyEl.classList.add('hidden');
    for (const e of data.entries) {
      const li = document.createElement('li');
      li.className = 'leaderboard-item';
      li.innerHTML = `<span class="lb-rank">#${e.rank}</span><span class="lb-score">${e.score}</span><span class="lb-code">${e.lobbyCode}</span>`;
      listEl.appendChild(li);
    }
  } catch {
    emptyEl.textContent = '加载失败，请稍后重试';
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
void load();
