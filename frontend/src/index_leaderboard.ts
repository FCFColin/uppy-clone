interface Entry {
  rank: number;
  score: number;
  lobbyCode: string;
}

export async function loadLeaderboardPreview(limit = 5): Promise<void> {
  const listEl = document.getElementById('leaderboard-preview');
  const emptyEl = document.getElementById('leaderboard-preview-empty');
  const errorEl = document.getElementById('leaderboard-preview-error');
  if (!listEl) return;

  try {
    const res = await fetch(`/api/v1/leaderboard?scope=global&limit=${limit}`);
    if (!res.ok) throw new Error('load failed');
    const data: { entries: Entry[] } = await res.json();
    listEl.textContent = '';
    if (!data.entries?.length) {
      emptyEl?.classList.remove('hidden');
      errorEl?.classList.add('hidden');
      return;
    }
    emptyEl?.classList.add('hidden');
    errorEl?.classList.add('hidden');
    for (const e of data.entries) {
      const li = document.createElement('li');
      li.className = 'leaderboard-item';
      li.innerHTML = `<span class="lb-rank">#${e.rank}</span><span class="lb-score">${e.score}</span><span class="lb-code">${e.lobbyCode}</span>`;
      listEl.appendChild(li);
    }
  } catch {
    if (emptyEl) emptyEl.classList.add('hidden');
    if (errorEl) {
      errorEl.textContent = '排行榜加载失败，可点击下方查看完整榜单';
      errorEl.classList.remove('hidden');
    }
  }
}
