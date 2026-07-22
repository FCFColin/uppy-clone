import { PALETTE_COLORS } from '../shared/game/constants.js';
import { endReasonLabel } from './constants.js';
import { truncateNickname } from './message_codec.js';
import type { GamePhase } from './state.js';
import { dispatch, getState } from './state.js';
import {
  $waitingScreen,
  $endedScreen,
  $gameHud,
  $cooldownIndicator,
  $hudScore,
  $hudPlayers,
  $finalScore,
  $hudPlayerList,
  $endPlayerList,
  $playerListWaiting,
  $nicknameSetupScreen,
  updateHudTimer,
  onGamePhaseChange,
} from './ui_common.js';
import { updateWindIndicator, hideWindIndicator } from './ui_common.js';
import { resizeCanvas } from './renderer.js';
import { isLowHeightDanger } from './visual_helpers.js';
import { isEntryHandoff } from './entry_flow.js';
import { getWaitingTitleText } from './entry_flow.js';
import { syncRestartVoteProgress } from './restart_vote_ui.js';
import { safeGetItem } from '../shared/ui/utils.js';

let lastPhase: GamePhase | null = null;
let lastPlayerListKey = '';
let lastEndListKey = '';

function playerListKey(): string {
  return getState()
    .players.map((p) => `${p.playerIndex}:${p.nickname}:${p.palette}`)
    .join('|');
}

function endListKey(): string {
  return getState()
    .players.map((p) => `${p.playerIndex}:${p.scoreContribution}:${p.nickname}:${p.palette}`)
    .sort()
    .join('|');
}

function isCurrentPlayer(p: { nickname: string }): boolean {
  const savedNickname: string | null = safeGetItem('uppy-nickname');
  const self = savedNickname || getState().pendingNickname || '';
  return self !== '' && p.nickname === self;
}

function isOverlayVisible(el: HTMLElement | null): boolean {
  return !!el && !el.classList.contains('hidden');
}

function setOverlayVisibility(): void {
  const phase = getState().phase;
  const playing = phase === 'playing';
  const handoff = isEntryHandoff();

  if (handoff) {
    $waitingScreen.classList.toggle('hidden', phase !== 'waiting');
  }

  $endedScreen.classList.toggle('hidden', phase !== 'ended');
  $gameHud.classList.toggle('hidden', !playing);
  $cooldownIndicator.classList.toggle('hidden', !playing);
  resizeCanvas();

  if (playing) {
    updateWindIndicator(getState().wind);
  } else {
    hideWindIndicator();
  }

  if (phase === 'waiting' && handoff) {
    const waitingTitle = document.getElementById('waiting-title');
    if (waitingTitle) {
      waitingTitle.textContent = getWaitingTitleText();
    }
  }

  if (handoff) {
    const hideNick = phase === 'countdown' || playing || phase === 'ended';
    if ($nicknameSetupScreen && hideNick) $nicknameSetupScreen.classList.add('hidden');
  }

  dispatch({
    type: 'SET_STATE',
    partial: {
      blockGameRender:
        isOverlayVisible($endedScreen) ||
        (isOverlayVisible($nicknameSetupScreen) && !getState().nicknameSubmitted) ||
        (isOverlayVisible($waitingScreen) && getState().nicknameSubmitted && getState().phase === 'waiting') ||
        isOverlayVisible(document.getElementById('tutorial-overlay')),
    },
  });
}

function displayNickname(p: { playerIndex: number; nickname: string }): string {
  const players = getState().players;
  const pending = getState().pendingNickname;
  const raw =
    pending && players.length === 1 && players[0]?.playerIndex === p.playerIndex
      ? pending
      : p.nickname || 'P' + p.playerIndex;
  // game-022: Truncate nickname for display to prevent layout overflow.
  // The backend also truncates, but the pending nickname (being typed) may
  // temporarily exceed the limit before the SET_NICKNAME message is sent.
  const name = truncateNickname(raw);
  if (isCurrentPlayer(p)) return `${name}（你）`;
  return name;
}

function renderPlayerItems(container: HTMLElement, includeScore: boolean, players = getState().players): void {
  container.textContent = '';
  for (const p of players) {
    const color: string = PALETTE_COLORS[p.palette % PALETTE_COLORS.length]!;
    const div: HTMLDivElement = document.createElement('div');
    div.className = 'player-item';
    const dot: HTMLSpanElement = document.createElement('span');
    dot.className = 'player-dot';
    dot.style.background = color;
    const name: HTMLSpanElement = document.createElement('span');
    name.className = 'player-name';
    name.textContent = displayNickname(p);
    div.appendChild(dot);
    div.appendChild(name);
    if (includeScore) {
      const score: HTMLSpanElement = document.createElement('span');
      score.className = 'player-score';
      score.textContent = String(p.scoreContribution);
      div.appendChild(score);
    }
    container.appendChild(div);
  }
}

function syncHudPlayerScores(): void {
  const items = $hudPlayerList.querySelectorAll('.player-item');
  getState().players.forEach((p, i) => {
    const item = items[i];
    if (!item) return;
    const scoreEl = item.querySelector('.player-score');
    if (scoreEl) scoreEl.textContent = String(p.scoreContribution);
  });
}

function syncHudOrWaitingPlayerList(force: boolean, phaseChanged: boolean): void {
  const pk = playerListKey();
  if (getState().phase === 'playing' || getState().phase === 'countdown') {
    if (force || phaseChanged || pk !== lastPlayerListKey) {
      lastPlayerListKey = pk;
      renderPlayerItems($hudPlayerList, true);
    } else {
      syncHudPlayerScores();
    }
    return;
  }
  if (getState().phase === 'waiting' && (force || phaseChanged || pk !== lastPlayerListKey)) {
    lastPlayerListKey = pk;
    renderPlayerItems($playerListWaiting, false);
  }
}

function renderEndPlayerList(force: boolean, phaseChanged: boolean): void {
  const ek = endListKey();
  if (!force && !phaseChanged && ek === lastEndListKey) return;
  lastEndListKey = ek;
  const sorted = [...getState().players].sort((a, b) => b.scoreContribution - a.scoreContribution);
  renderPlayerItems($endPlayerList, true, sorted);
}

function updateScoreHud(): void {
  $hudScore.textContent = String(getState().score);
  $hudScore.classList.toggle('score-danger', isLowHeightDanger());
  $hudPlayers.textContent = String(getState().players.length);
}

export function updateUI(opts?: { force?: boolean }): void {
  const phaseChanged = getState().phase !== lastPhase;
  const force = opts?.force ?? false;
  if (phaseChanged || force) {
    lastPhase = getState().phase;
    setOverlayVisibility();
    if (phaseChanged) onGamePhaseChange();
  }

  updateScoreHud();
  updateHudTimer();

  if (getState().phase === 'ended') {
    $finalScore.textContent = String(getState().score);
    const reasonEl = document.getElementById('end-reason');
    if (reasonEl) {
      const endReason = getState().endReason;
      const label = endReason != null ? endReasonLabel(endReason) : '';
      reasonEl.textContent = label;
      reasonEl.style.display = label ? 'block' : 'none';
    }
    renderEndPlayerList(force, phaseChanged);
  }

  if (getState().phase === 'ended' && getState().restartVotes) {
    syncRestartVoteProgress();
    if (getState().restartVotes.countdownMs <= 0) {
      const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
      if ($restartCountdown) $restartCountdown.textContent = '';
    }
  }

  syncHudOrWaitingPlayerList(force, phaseChanged);
}

export function updateScoresOnly(): void {
  updateScoreHud();

  syncHudOrWaitingPlayerList(false, false);

  if (getState().phase === 'ended') {
    $finalScore.textContent = String(getState().score);
    renderEndPlayerList(true, false);
  }
}

/** Reset UI update cache for a new game session. */
export function resetUIUpdateCache(): void {
  lastPhase = null;
  lastPlayerListKey = '';
  lastEndListKey = '';
}
