import type { GamePhase } from '../shared/types.js';
import {
  state, resetInterpolation, freezeInterpolation,
  seenSeqs,
} from './state.js';
import {
  updateUI, startCountdownTimer,
  hideCountdownOverlay, showCountdownOverlay,
} from './ui.js';

/**
 * Whether a snapshot phase transition is allowed from the current client phase.
 *
 * After a server restart the client may be stuck in a stale phase (e.g. 'playing')
 * while the server has already moved to 'waiting' or started a new 'countdown'.
 * We must allow every forward/backward transition so the client can re-sync.
 */
export function shouldApplySnapshotPhase(snapshotPhase: GamePhase): boolean {
  const client = state.phase;
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

/**
 * Clear per-round client FX without wiping snapshot/render readiness.
 * Also resets score and entity positions so that a dropped first snapshot
 * (e.g. due to seq collision after restart) doesn't leave stale state.
 */
function resetRoundClientState(): void {
  state.ripples = [];
  state.explosionEffect = null;
  state.myCooldownEnd = 0;
  state.lastTapX = null;
  state.lastTapY = null;
  state.restartClicked = false;
  state.restartVotes = { yes: 0, total: 0, countdownMs: 0 };
  state.score = 0;
  state.balloon = { x: 0.5, y: 0.95, vx: 0, vy: 0 };
  state.bird = { x: 0, y: 0, active: false };
  state.ghost = { x: 0, y: 0, active: false, repelTimer: 0 };
  state.wind = 0;
}

/** Hide nickname UI once the round begins — nickname belongs to setup only. */
function hideNicknameUI(): void {
  const setup: HTMLElement | null = document.getElementById('nickname-setup-screen');
  const inline: HTMLElement | null = document.getElementById('nickname-inline');
  if (setup) setup.classList.add('hidden');
  if (inline) inline.classList.add('hidden');
}

/**
 * Apply side effects when game phase changes (from GAME_STATE_CHANGE or snapshot).
 * Returns true when the phase actually changed.
 */
export function applyPhaseChange(nextPhase: GamePhase, countdownSeconds = 3): boolean {
  const prevPhase = state.phase;
  if (nextPhase === prevPhase) return false;

  // Don't enter gameplay until the player clicked "进入游戏" (restart from ended is OK).
  if (
    !state.nicknameSubmitted
    && prevPhase === 'waiting'
    && (nextPhase === 'countdown' || nextPhase === 'playing')
  ) {
    return false;
  }

  state.phase = nextPhase;
  window.__gamePhase = nextPhase;

  if (nextPhase === 'playing') {
    resetRoundClientState();
    // Clear seenSeqs so the first playing snapshot (which may share the same
    // tick-count/seq as the preceding countdown snapshot) is not dropped as a
    // duplicate. This is critical after a restart where TickCount resets to 0.
    seenSeqs.clear();
    hideCountdownOverlay();
    if (state.countdownTimerInterval !== null) {
      clearInterval(state.countdownTimerInterval);
      state.countdownTimerInterval = null;
    }
    if (window._restartCountdownTimer) {
      clearInterval(window._restartCountdownTimer);
      window._restartCountdownTimer = null;
    }
    hideNicknameUI();
    resetInterpolation();
  } else if (nextPhase === 'countdown') {
    resetRoundClientState();
    seenSeqs.clear();
    hideNicknameUI();
    resetInterpolation();
    showCountdownOverlay();
    startCountdownTimer(countdownSeconds);
  } else if (nextPhase === 'ended') {
    hideCountdownOverlay();
    hideNicknameUI();
    freezeInterpolation();
    state.restartVotes = { yes: 0, total: state.players.length, countdownMs: 0 };
  } else if (nextPhase === 'waiting') {
    hideCountdownOverlay();
  }

  updateUI(true);
  return true;
}
