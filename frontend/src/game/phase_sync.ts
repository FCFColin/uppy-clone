import type { GamePhase } from '../shared/game/types.js';
import { state } from './state_types.js';
import { resetInterpolation, freezeInterpolation, seenSeqs } from './state_interp.js';
import { resetRoundClientState } from './state_reset.js';
import {
  updateUI, startCountdownTimer,
  hideCountdownOverlay, showCountdownOverlay,
  startCooldownUpdater, stopCooldownUpdater,
} from './ui.js';
import { tryEntryHandoff } from './entry_flow.js';

export const END_REASON = {
  NONE: 0,
  GROUND: 1,
  BIRD: 2,
  GHOST: 3,
} as const;

const END_REASON_LABELS: Record<number, string> = {
  [END_REASON.GROUND]: '气球落地',
  [END_REASON.BIRD]: '被鸟撞到',
  [END_REASON.GHOST]: '被幽灵碰到',
};

export function endReasonLabel(code: number): string {
  return END_REASON_LABELS[code] ?? '';
}

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

/** Hide nickname UI once the round begins — nickname belongs to setup only. */
function hideNicknameUI(): void {
  const setup: HTMLElement | null = document.getElementById('nickname-setup-screen');
  const inline: HTMLElement | null = document.getElementById('nickname-inline');
  if (setup) setup.classList.add('hidden');
  if (inline) inline.classList.add('hidden');
}

function clearRestartCountdownTimer(): void {
  if (state.countdownTimerInterval !== null) {
    clearInterval(state.countdownTimerInterval);
    state.countdownTimerInterval = null;
  }
  if (window._restartCountdownTimer) {
    clearInterval(window._restartCountdownTimer);
    window._restartCountdownTimer = null;
  }
}

function onEnterPlaying(): void {
  state.endReason = null;
  resetRoundClientState();
  seenSeqs.clear();
  hideCountdownOverlay();
  clearRestartCountdownTimer();
  hideNicknameUI();
  resetInterpolation();
  startCooldownUpdater();
}

function onEnterCountdown(countdownSeconds: number): void {
  stopCooldownUpdater();
  resetRoundClientState();
  seenSeqs.clear();
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
  state.restartVotes = { yes: 0, total: state.players.length, countdownMs: 0 };
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
  const prevPhase = state.phase;
  if (nextPhase === prevPhase) return false;

  // Don't enter gameplay (or ended) until the player clicked「进入游戏」.
  if (
    !state.nicknameSubmitted
    && prevPhase === 'waiting'
    && (nextPhase === 'countdown' || nextPhase === 'playing' || nextPhase === 'ended')
  ) {
    return false;
  }

  state.phase = nextPhase;
  window.__gamePhase = nextPhase;

  tryEntryHandoff(nextPhase);
  phaseEnterHooks[nextPhase](countdownSeconds);

  updateUI(true);
  return true;
}
