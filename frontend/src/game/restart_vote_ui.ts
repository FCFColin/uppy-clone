import { state } from './state.js';

export function syncRestartVoteProgress(): void {
  if (state.phase !== 'ended' || !state.restartVotes) return;

  const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
  if (!$restartProgress) return;

  const { yes, total } = state.restartVotes;
  if (yes >= total && total > 0) {
    $restartProgress.textContent = '正在重启游戏...';
  } else {
    const need = total - yes;
    $restartProgress.textContent = `${yes}/${total} 人已投票，还差 ${need} 人`;
  }
}

export function syncRestartVoteCountdown(): void {
  if (!state.restartVotes || state.restartVotes.countdownMs <= 0) {
    const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
    if ($restartCountdown) $restartCountdown.textContent = '';
    return;
  }

  if (window._restartCountdownTimer) return;

  window._restartCountdownTimer = setInterval(() => {
    if (!state.restartVotes || state.restartVotes.countdownMs <= 0) {
      clearInterval(window._restartCountdownTimer!);
      window._restartCountdownTimer = null;
      return;
    }
    const elapsed: number = Date.now() - (state.restartVotes.receivedAt ?? 0);
    const remaining: number = Math.max(0, state.restartVotes.countdownMs - elapsed);
    const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
    if ($restartCountdown && remaining > 0) {
      $restartCountdown.textContent = `${Math.ceil(remaining / 1000)} 秒后自动重启`;
    } else if ($restartCountdown) {
      $restartCountdown.textContent = '';
      clearInterval(window._restartCountdownTimer!);
      window._restartCountdownTimer = null;
    }
  }, 1000);
}

export function syncRestartVoteUI(): void {
  syncRestartVoteCountdown();
  syncRestartVoteProgress();
}
