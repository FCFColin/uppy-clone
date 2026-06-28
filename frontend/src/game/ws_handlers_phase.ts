import { codeToPhase } from './message_codec.js';
import { state } from './state.js';
import { applyPhaseChange, shouldApplySnapshotPhase } from './phase_sync.js';
import { updateUI } from './ui.js';
import { runTutorialIfNeeded } from './tutorial.js';
import { playGameOverSound, vibrate } from '../shared/audio.js';
import { updateBestScore, fetchUserBestScore } from '../shared/best_score_cookie.js';
import { syncRestartVoteUI } from './restart_vote_ui.js';

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
    yes,
    total,
    countdownMs,
    receivedAt: Date.now(),
  };
  syncRestartVoteUI();
  updateUI(true);
}
