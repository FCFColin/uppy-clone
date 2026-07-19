import type { GamePhase } from './state.js';
import { dispatch, getState } from './state.js';
import { resetInterpolation, freezeInterpolation } from './state_interp.js';
import { clearSeenSeqs } from './seen_seqs.js';
import { resetRoundClientState } from './state_interp.js';
import { updateUI } from './ui_update.js';
import { startCountdownTimer, hideCountdownOverlay, showCountdownOverlay, startCooldownUpdater, stopCooldownUpdater } from './ui_common.js';
import { clearRestartCountdownTimer as clearVoteCountdownTimer } from './restart_vote_ui.js';
import { tryEntryHandoff } from './entry_flow.js';
/**
 * Whether a snapshot phase transition is allowed from the current client phase.
 *
 * After a server restart the client may be stuck in a stale phase (e.g. 'playing')
 * while the server has already moved to 'waiting' or started a new 'countdown'.
 * We must allow every forward/backward transition so the client can re-sync.
 */
export function shouldApplySnapshotPhase(snapshotPhase: GamePhase): boolean {
  const client = getState().phase;
  if (snapshotPhase === client) return true;

  switch (client) {
    case 'waiting':
      return snapshotPhase === 'countdown' || snapshotPhase === 'playing' || snapshotPhase === 'ended';
    case 'countdown':
      return snapshotPhase === 'playing';
    case 'playing':
      // Allow transition to 'ended' (normal game over), 'countdown' (new round
      // after server restart or auto-restart), and 'waiting' (server reset).
      return snapshotPhase === 'ended' || snapshotPhase === 'countdown' || snapshotPhase === 'waiting';
    case 'ended':
      return snapshotPhase === 'countdown' || snapshotPhase === 'waiting';
    default:
      return true;
  }
}

/** Hide nickname UI once the round begins — nickname belongs to setup only. */
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
  dispatch({ type: 'SET_STATE', partial: { restartVotes: { yes: 0, total: getState().players.length, countdownMs: 0 } } });
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

/**
 * Apply side effects when game phase changes (from GAME_STATE_CHANGE or snapshot).
 * Returns true when the phase actually changed.
 */
export function applyPhaseChange(nextPhase: GamePhase, countdownSeconds = 3): boolean {
  const prevPhase = getState().phase;
  if (nextPhase === prevPhase) return false;

  // Don't enter gameplay (or ended) until the player clicked「进入游戏」.
  if (
    !getState().nicknameSubmitted
    && prevPhase === 'waiting'
    && (nextPhase === 'countdown' || nextPhase === 'playing' || nextPhase === 'ended')
  ) {
    return false;
  }

  dispatch({ type: 'SET_STATE', partial: { phase: nextPhase } });
  // shared-016: Only expose debug global in dev mode to avoid leaking
  // internal state in production builds.
  if (import.meta.env.DEV) {
    window.__gamePhase = nextPhase;
  }

  tryEntryHandoff(nextPhase);
  phaseEnterHooks[nextPhase](countdownSeconds);

  updateUI({ force: true });
  return true;
}
