import type { GamePhase } from './state.js';
import { dispatch, getState } from './state.js';
import { resetInterpolation, freezeInterpolation, resetRoundClientState, clearSeenSeqs } from './state_interp.js';
import { updateUI } from './ui_update.js';
import {
  startCountdownTimer,
  hideCountdownOverlay,
  showCountdownOverlay,
  startCooldownUpdater,
  stopCooldownUpdater,
} from './ui_common.js';
import { clearRestartCountdownTimer as clearVoteCountdownTimer } from './restart_vote_ui.js';
import { tryEntryHandoff } from './entry_flow.js';
export function shouldApplySnapshotPhase(_snapshotPhase: GamePhase): boolean {
  return true;
}

function hideNicknameUI(): void {
  const setup: HTMLElement | null = document.getElementById('nickname-setup-screen');
  const inline: HTMLElement | null = document.getElementById('nickname-inline');
  if (setup) setup.classList.add('hidden');
  if (inline) inline.classList.add('hidden');
}

function clearRestartCountdownTimer(): void {
  const interval = getState().countdownTimerInterval;
  if (interval !== null) {
    clearInterval(interval);
    dispatch({ type: 'SET_STATE', partial: { countdownTimerInterval: null } });
  }
  clearVoteCountdownTimer();
}

function onEnterPlaying(): void {
  dispatch({ type: 'SET_END_REASON', reason: null });
  resetRoundClientState();
  clearSeenSeqs();
  hideCountdownOverlay();
  clearRestartCountdownTimer();
  hideNicknameUI();
  resetInterpolation();
  startCooldownUpdater();
}

function onEnterCountdown(countdownSeconds: number): void {
  stopCooldownUpdater();
  resetRoundClientState();
  clearSeenSeqs();
  hideNicknameUI();
  resetInterpolation();
  showCountdownOverlay();
  startCountdownTimer(countdownSeconds);
}

function onEnterEnded(): void {
  stopCooldownUpdater();
  hideCountdownOverlay();
  hideNicknameUI();
  freezeInterpolation();
  dispatch({
    type: 'SET_STATE',
    partial: { restartVotes: { yes: 0, total: getState().players.length, countdownMs: 0 } },
  });
}

function onEnterWaiting(): void {
  stopCooldownUpdater();
  hideCountdownOverlay();
}

const phaseEnterHooks: Record<GamePhase, (countdownSeconds: number) => void> = {
  playing: () => onEnterPlaying(),
  countdown: (countdownSeconds) => onEnterCountdown(countdownSeconds),
  ended: () => onEnterEnded(),
  waiting: () => onEnterWaiting(),
};

export function applyPhaseChange(nextPhase: GamePhase, countdownSeconds = 3): boolean {
  const prevPhase = getState().phase;
  if (nextPhase === prevPhase) return false;

  if (
    !getState().nicknameSubmitted &&
    prevPhase === 'waiting' &&
    (nextPhase === 'countdown' || nextPhase === 'playing' || nextPhase === 'ended')
  ) {
    return false;
  }

  dispatch({ type: 'SET_STATE', partial: { phase: nextPhase } });
  // shared-016: Only expose debug global in dev mode to avoid leaking
  if (import.meta.env.DEV) {
    window.__gamePhase = nextPhase;
  }

  tryEntryHandoff(nextPhase);
  phaseEnterHooks[nextPhase](countdownSeconds);

  updateUI({ force: true });
  return true;
}
