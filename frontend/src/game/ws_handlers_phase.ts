import { codeToPhase } from './message_codec.js';
import { state } from './state.js';
import { applyPhaseChange, shouldApplySnapshotPhase } from './phase_sync.js';
import { updateUI } from './ui.js';
import { runTutorialIfNeeded } from './tutorial.js';
import { playGameOverSound, vibrate } from '../shared/audio.js';
import { updateBestScore, fetchUserBestScore } from '../shared/best_score_cookie.js';

export function handleGameStateChange(view: DataView): void {
  const phaseCode: number = view.getUint8(1);
  const nextPhase = codeToPhase(phaseCode);
  console.log(`[game-state-change] newPhase=${nextPhase} prevPhase=${state.phase}`);

  if (nextPhase === 'ended' && view.byteLength >= 3) {
    state.endReason = view.getUint8(2);
    playGameOverSound();
    vibrate(200);
    void updateEndScreenRecords();
  }

  if (!shouldApplySnapshotPhase(nextPhase)) return;

  let countdownSeconds = 3;
  if (nextPhase === 'countdown' && view.byteLength >= 6) {
    const remainingMs: number = view.getUint32(2, true);
    countdownSeconds = Math.max(1, Math.ceil(remainingMs / 1000));
  }

  if (nextPhase === 'countdown') {
    void runTutorialIfNeeded().then(() => {
      applyPhaseChange(nextPhase, countdownSeconds);
    });
    return;
  }

  applyPhaseChange(nextPhase, countdownSeconds);
}

async function updateEndScreenRecords(): Promise<void> {
  const bestEl = document.getElementById('personal-best');
  if (!bestEl) return;
  const score = state.score;
  const cookieBest = updateBestScore(score);
  let best = cookieBest.best;
  try {
    const serverBest = await fetchUserBestScore();
    if (serverBest > best) best = serverBest;
  } catch { /* use cookie */ }
  const parts = [`本局 ${score}`, `个人最佳 ${Math.max(best, score)}`];
  if (cookieBest.isNewRecord || score > best) parts.push('新纪录！');
  bestEl.textContent = parts.join(' · ');
  updateUI(true);
}

export function handleRestartStatus(view: DataView): void {
  const yes: number = view.getUint8(1);
  const total: number = view.getUint8(2);
  const countdownMs: number = view.getUint32(3, true);
  state.restartVotes = {
    yes: yes,
    total: total,
    countdownMs: countdownMs,
    receivedAt: Date.now(),
  };
  if (countdownMs > 0 && !window._restartCountdownTimer) {
    window._restartCountdownTimer = setInterval(() => {
      if (state.restartVotes && state.restartVotes.countdownMs > 0) {
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
      }
    }, 1000);
  }

  const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
  if ($restartProgress && state.phase === 'ended') {
    if (yes >= total && total > 0) {
      $restartProgress.textContent = '正在重启游戏...';
    } else {
      const need = total - yes;
      $restartProgress.textContent = `${yes}/${total} 人已投票，还差 ${need} 人`;
    }
  }

  updateUI(true);
}
