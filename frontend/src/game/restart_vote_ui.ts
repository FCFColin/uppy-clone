import { getState } from './store.js';

export function syncRestartVoteProgress(): void {
  const s = getState();
  if (s.phase !== 'ended' || !s.restartVotes) return;

  const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
  if (!$restartProgress) return;

  const { yes, total } = s.restartVotes;
  if (yes >= total && total > 0) {
    $restartProgress.textContent = '正在重启游戏...';
  } else {
    const need = total - yes;
    $restartProgress.textContent = `${yes}/${total} 人已投票，还差 ${need} 人`;
  }
}

let restartCountdownTimer: ReturnType<typeof setInterval> | null = null;

function noActiveCountdown(): boolean {
  return !getState().restartVotes || getState().restartVotes.countdownMs <= 0;
}

export function syncRestartVoteCountdown(): void {
  if (noActiveCountdown()) {
    const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
    if ($restartCountdown) $restartCountdown.textContent = '';
    return;
  }

  if (restartCountdownTimer) return;

  restartCountdownTimer = setInterval(() => {
    if (noActiveCountdown()) {
      clearInterval(restartCountdownTimer!);
      restartCountdownTimer = null;
      return;
    }
    const elapsed: number = Date.now() - (getState().restartVotes.receivedAt ?? 0);
    const remaining: number = Math.max(0, getState().restartVotes.countdownMs - elapsed);
    const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
    if ($restartCountdown && remaining > 0) {
      $restartCountdown.textContent = `${Math.ceil(remaining / 1000)} 秒后自动重启`;
    } else if ($restartCountdown) {
      $restartCountdown.textContent = '';
      clearInterval(restartCountdownTimer!);
      restartCountdownTimer = null;
    }
  }, 1000);
}

export function syncRestartVoteUI(): void {
  syncRestartVoteCountdown();
  syncRestartVoteProgress();
}
