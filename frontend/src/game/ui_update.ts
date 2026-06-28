import { PALETTE_COLORS } from './constants.js';
import type { GamePhase } from '../shared/types.js';
import { state } from './state.js';
import {
  $waitingScreen, $endedScreen, $gameHud, $cooldownIndicator,
  $hudScore, $hudPlayers, $finalScore, $hudPlayerList,
  $endPlayerList, $playerListWaiting,
  $nicknameSetupScreen,
} from './ui_elements.js';
import { updateWindIndicator, hideWindIndicator } from './ui_wind.js';
import { endReasonLabel } from './end_reason.js';
import { refreshLayout } from './ui.js';
import { isLowHeightDanger } from './visual_helpers.js';
import { isEntryHandoff, getWaitingTitleText } from './entry_flow.js';
import { syncRestartVoteProgress } from './restart_vote_ui.js';

let lastPhase: GamePhase | null = null;
let lastPlayerListKey = '';
let lastEndListKey = '';

function playerListKey(): string {
  return state.players
    .map(p => `${p.playerIndex}:${p.nickname}:${p.palette}`)
    .join('|');
}

function endListKey(): string {
  return state.players
    .map(p => `${p.playerIndex}:${p.scoreContribution}:${p.nickname}:${p.palette}`)
    .sort()
    .join('|');
}

function isCurrentPlayer(p: { nickname: string }): boolean {
  const saved = localStorage.getItem('uppy-nickname') || state.pendingNickname || '';
  return saved !== '' && p.nickname === saved;
}

function setOverlayVisibility(): void {
  if (isEntryHandoff()) {
    $waitingScreen.classList.toggle('hidden', state.phase !== 'waiting');
  }

  $endedScreen.classList.toggle('hidden', state.phase !== 'ended');
  $gameHud.classList.toggle('hidden', state.phase !== 'playing');
  $cooldownIndicator.classList.toggle('hidden', state.phase !== 'playing');
  refreshLayout();

  if (state.phase === 'playing') {
    updateWindIndicator(state.wind);
  } else {
    hideWindIndicator();
  }

  if (state.phase === 'waiting' && isEntryHandoff()) {
    const waitingTitle: HTMLElement | null = document.getElementById('waiting-title');
    if (waitingTitle) {
      waitingTitle.textContent = getWaitingTitleText();
    }
  }

  if (isEntryHandoff()) {
    const hideNick = state.phase === 'countdown' || state.phase === 'playing' || state.phase === 'ended';
    if ($nicknameSetupScreen && hideNick) $nicknameSetupScreen.classList.add('hidden');
  }
}

function displayNickname(p: { playerIndex: number; nickname: string }): string {
  let name: string;
  if (
    state.pendingNickname
    && state.players.length === 1
    && state.players[0]?.playerIndex === p.playerIndex
  ) {
    name = state.pendingNickname;
  } else {
    name = p.nickname || 'P' + p.playerIndex;
  }
  if (isCurrentPlayer(p)) return `${name}（你）`;
  return name;
}

function renderPlayerItems(
  container: HTMLElement,
  includeScore: boolean,
  players = state.players,
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
  state.players.forEach((p, i) => {
    const item = items[i];
    if (!item) return;
    const scoreEl = item.querySelector('.player-score');
    if (scoreEl) scoreEl.textContent = String(p.scoreContribution);
  });
}

function syncHudOrWaitingPlayerList(force: boolean, phaseChanged: boolean): void {
  const pk = playerListKey();
  if (state.phase === 'playing' || state.phase === 'countdown') {
    if (force || phaseChanged || pk !== lastPlayerListKey) {
      lastPlayerListKey = pk;
      renderPlayerItems($hudPlayerList, true);
    } else {
      syncHudPlayerScores();
    }
    return;
  }
  if (state.phase === 'waiting' && (force || phaseChanged || pk !== lastPlayerListKey)) {
    lastPlayerListKey = pk;
    renderPlayerItems($playerListWaiting, false);
  }
}

function renderEndPlayerList(force: boolean, phaseChanged: boolean): void {
  const ek = endListKey();
  if (!force && !phaseChanged && ek === lastEndListKey) return;
  lastEndListKey = ek;
  const sorted = [...state.players].sort((a, b) => b.scoreContribution - a.scoreContribution);
  renderPlayerItems($endPlayerList, true, sorted);
}

export function updateUI(force = false): void {
  const phaseChanged = state.phase !== lastPhase;
  if (phaseChanged || force) {
    lastPhase = state.phase;
    setOverlayVisibility();
  }

  $hudScore.textContent = String(state.score);
  $hudScore.classList.toggle('score-danger', isLowHeightDanger());
  $hudPlayers.textContent = String(state.players.length);

  if (state.phase === 'ended') {
    $finalScore.textContent = String(state.score);
    const reasonEl = document.getElementById('end-reason');
    if (reasonEl) {
      const label = state.endReason != null ? endReasonLabel(state.endReason) : '';
      reasonEl.textContent = label;
      reasonEl.style.display = label ? 'block' : 'none';
    }
    renderEndPlayerList(force, phaseChanged);
  }

  if (state.phase === 'ended' && state.restartVotes) {
    syncRestartVoteProgress();
    if (state.restartVotes.countdownMs <= 0) {
      const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
      if ($restartCountdown) $restartCountdown.textContent = '';
    }
  }

  syncHudOrWaitingPlayerList(force, phaseChanged);
}

export function updateScoresOnly(): void {
  $hudScore.textContent = String(state.score);
  $hudScore.classList.toggle('score-danger', isLowHeightDanger());
  $hudPlayers.textContent = String(state.players.length);

  syncHudOrWaitingPlayerList(false, false);

  if (state.phase === 'ended') {
    $finalScore.textContent = String(state.score);
    renderEndPlayerList(true, false);
  }
}
