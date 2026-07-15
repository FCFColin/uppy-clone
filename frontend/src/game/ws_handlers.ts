import { MSG_TYPE } from '../shared/game/protocol.js';
import { handlePong } from './ws_connection.js';
import { dispatch, getState } from './store.js';
import { pushFloatingText } from './visual_helpers.js';
import { codeToPhase, applySnapshot, decodeSnapshot } from './message_codec.js';
import { applyPhaseChange, shouldApplySnapshotPhase } from './phase_sync.js';
import { updateUI, updateScoresOnly } from './ui_update.js';
import { updateWindIndicator } from './ui_wind.js';
import { runTutorialIfNeeded } from './tutorial.js';
import { playGameOverSound, vibrate } from '../shared/ui/audio.js';
import { updateBestScore, fetchUserBestScore } from '../shared/data/best_score_cookie.js';
import { syncRestartVoteUI } from './restart_vote_ui.js';
import { updateInterpolation, freezeInterpolation } from './state_interp.js';
import { isDuplicateSeq } from './seen_seqs.js';

export { shouldApplySnapshotPhase } from './phase_sync.js';

// ─── Binary Message Dispatcher ───────────────────────────────────────

export function handleBinaryMessage(buffer: ArrayBuffer): void {
  if (buffer.byteLength < 1) {
    console.warn('[ws] empty binary message, ignoring');
    return;
  }
  const view: DataView = new DataView(buffer);
  const msgType: number = view.getUint8(0);

  switch (msgType) {
    case MSG_TYPE.SNAPSHOT:
      handleSnapshot(view);
      break;
    case MSG_TYPE.TAP_ACCEPTED:
      handleTapAccepted(view);
      break;
    case MSG_TYPE.TAP_REJECTED:
      handleTapRejected();
      break;
    case MSG_TYPE.GAME_STATE_CHANGE:
      handleGameStateChange(view);
      break;
    case MSG_TYPE.RESTART_STATUS:
      handleRestartStatus(view);
      break;
    case MSG_TYPE.PONG:
      handlePong();
      break;
    case MSG_TYPE.PLAYER_JOIN:
    case MSG_TYPE.PLAYER_LEAVE:
      break;
    default:
      console.warn('Unknown message type:', msgType);
  }
}

// ─── Snapshot ────────────────────────────────────────────────────────

export function handleSnapshot(view: DataView): void {
  try {
    if (view.byteLength < 37) {
      console.warn('[snapshot] message too short, ignoring');
      return;
    }

    const decoded = decodeSnapshot(view);
    if (!decoded) return;

    if (isDuplicateSeq(decoded.timestamp)) return;

    const optimisticRipples = getState().ripples.filter(r => r.isOptimistic);
    const snapshotUpdate = applySnapshot(decoded);
    dispatch({ type: 'SET_STATE', partial: snapshotUpdate });

    if (shouldApplySnapshotPhase(decoded.phase)) {
      applyPhaseChange(decoded.phase);
    }

    if (decoded.ripples.length > 0) {
      dispatch({ type: 'SET_STATE', partial: {
        ripples: [...optimisticRipples, ...decoded.ripples],
      }});
    }

    if (decoded.wind !== undefined) {
      dispatch({ type: 'SET_STATE', partial: { wind: decoded.wind } });
      updateWindIndicator(decoded.wind);
    }

    updateScoresOnly();
    if (getState().pendingNickname) {
      const matched = getState().players.some((p) => p.nickname === getState().pendingNickname);
      if (matched) dispatch({ type: 'SET_STATE', partial: { pendingNickname: null } });
    }
    if (getState().phase === 'ended') {
      freezeInterpolation();
    } else {
      updateInterpolation(decoded.timestamp);
    }
    dispatch({ type: 'SET_STATE', partial: { hasReceivedFirstSnapshot: true } });
  } catch (e: unknown) {
    const errMsg: string = e instanceof Error ? e.message : String(e);
    console.error('[snapshot] parse error:', errMsg);
  }
}

// ─── Phase Change / Restart Status ───────────────────────────────────

export function handleGameStateChange(view: DataView): void {
  if (view.byteLength < 2) {
    console.warn('[ws] GAME_STATE_CHANGE too short, ignoring');
    return;
  }
  const phaseCode: number = view.getUint8(1);
  const nextPhase = codeToPhase(phaseCode);

  if (!shouldApplySnapshotPhase(nextPhase)) return;

  if (nextPhase === 'ended' && view.byteLength >= 3) {
    dispatch({ type: 'SET_END_REASON', reason: view.getUint8(2) });
    playGameOverSound();
    vibrate(200);
    void updateEndScreenRecords();
  }

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
  const score = getState().score;
  const cookieBest = updateBestScore(score);
  let best = cookieBest.best;
  try {
    const serverBest = await fetchUserBestScore();
    if (serverBest > best) best = serverBest;
  } catch { /* use cookie */ }
  const parts = [`本局 ${score}`, `个人最佳 ${Math.max(best, score)}`];
  if (cookieBest.isNewRecord || score > best) parts.push('新纪录！');
  bestEl.textContent = parts.join(' · ');
  updateUI({ force: true });
}

export function handleRestartStatus(view: DataView): void {
  if (view.byteLength < 7) {
    console.warn('[ws] RESTART_STATUS too short, ignoring');
    return;
  }
  const yes: number = view.getUint8(1);
  const total: number = view.getUint8(2);
  const countdownMs: number = view.getUint32(3, true);
  dispatch({ type: 'SET_STATE', partial: {
    restartVotes: { yes, total, countdownMs, receivedAt: Date.now() },
  }});
  syncRestartVoteUI();
  updateUI({ force: true });
}

// ─── Tap Events ──────────────────────────────────────────────────────

export function handleTapAccepted(view: DataView): void {
  if (view.byteLength < 15) {
    console.warn('[ws] TAP_ACCEPTED too short, ignoring');
    return;
  }
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const cooldownRemainingMs: number = view.getUint32(o, true); o += 4;
  const _balloonX: number = view.getFloat32(o, true); o += 4;
  const _balloonY: number = view.getFloat32(o, true);
  dispatch({ type: 'SET_STATE', partial: { myCooldownEnd: Date.now() + cooldownRemainingMs } });
  const tapX = getState().lastTapX ?? _balloonX;
  const tapY = getState().lastTapY ?? _balloonY;
  dispatch({ type: 'SET_STATE', partial: {
    ripples: [...getState().ripples.filter(r => !r.isOptimistic), { playerIndex, x: tapX, y: tapY, time: Date.now() }],
    explosionEffect: { x: tapX, y: tapY, startTime: Date.now() },
  }});
}

export function handleTapRejected(): void {
  const lastTapX = getState().lastTapX;
  const lastTapY = getState().lastTapY;
  const remaining = getState().ripples.filter(r => !r.isOptimistic);
  if (lastTapX !== null && lastTapY !== null) {
    remaining.push({
      playerIndex: -1,
      x: lastTapX,
      y: lastTapY,
      time: Date.now(),
      rejected: true,
    });
    pushFloatingText(lastTapX, lastTapY, '太远了');
  }
  dispatch({ type: 'SET_STATE', partial: { myCooldownEnd: 0, ripples: remaining } });
}
