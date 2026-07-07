import { PALETTE_COLORS, endReasonLabel } from './local_constants.js';
import type { GamePhase } from '../shared/game/types.js';
import { dispatch, getState } from './store.js';
import { state } from './state_types.js';
import {
  $waitingScreen, $endedScreen, $gameHud, $cooldownIndicator,
  $hudScore, $hudPlayers, $finalScore, $hudPlayerList,
  $endPlayerList, $playerListWaiting,
  $nicknameSetupScreen,
} from './ui_elements.js';
import { updateWindIndicator, hideWindIndicator } from './ui_wind.js';
import { refreshLayout } from './ui_utils.js';
import { isLowHeightDanger } from './visual_helpers.js';
import { isEntryHandoff, getWaitingTitleText } from './entry_flow.js';
import { syncRestartVoteProgress } from './restart_vote_ui.js';

let lastPhase: GamePhase | null = null;
let lastPlayerListKey = '';
let lastEndListKey = '';

function playerListKey(): string {
  return getState().players
    .map(p => `${p.playerIndex}:${p.nickname}:${p.palette}`)
    .join('|');
}

function endListKey(): string {
  return getState().players
    .map(p => `${p.playerIndex}:${p.scoreContribution}:${p.nickname}:${p.palette}`)
    .sort()
    .join('|');
}

let _savedNickname: string | null = null;

function isCurrentPlayer(p: { nickname: string }): boolean {
  if (_savedNickname === null) {
    _savedNickname = localStorage.getItem('uppy-nickname') || getState().pendingNickname || '';
  }
  return _savedNickname !== '' && p.nickname === _savedNickname;
}

export function invalidateNicknameCache(): void {
  _savedNickname = null;
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
  refreshLayout();

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
        (isOverlayVisible($nicknameSetupScreen) && !state.nicknameSubmitted) ||
        (isOverlayVisible($waitingScreen) && state.nicknameSubmitted && state.phase === 'waiting') ||
        isOverlayVisible(document.getElementById('tutorial-overlay')),
    },
  });
}

function displayNickname(p: { playerIndex: number; nickname: string }): string {
  const players = getState().players;
  const pending = getState().pendingNickname;
  const name = (pending && players.length === 1 && players[0]?.playerIndex === p.playerIndex)
    ? pending
    : (p.nickname || 'P' + p.playerIndex);
  if (isCurrentPlayer(p)) return `${name}（你）`;
  return name;
}

function renderPlayerItems(
  container: HTMLElement,
  includeScore: boolean,
  players = getState().players,
): void {
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
  }

  updateScoreHud();

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
