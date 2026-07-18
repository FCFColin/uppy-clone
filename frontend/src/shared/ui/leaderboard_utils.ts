export interface LeaderboardEntry {
  rank: number;
  score: number;
  lobbyCode: string;
}

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
