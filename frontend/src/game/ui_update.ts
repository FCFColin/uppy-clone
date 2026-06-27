import { PALETTE_COLORS } from './constants.js';
import type { GamePhase } from '../shared/types.js';
import { state } from './state.js';
import {
  $waitingScreen, $endedScreen, $gameHud, $cooldownIndicator,
  $hudScore, $hudPlayers, $finalScore, $hudPlayerList,
  $endPlayerList, $playerListWaiting,
  $nicknameSetupScreen, $nicknameInline,
} from './ui_elements.js';

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

function setOverlayVisibility(): void {
  $waitingScreen.classList.toggle('hidden', state.phase !== 'waiting');
  $endedScreen.classList.toggle('hidden', state.phase !== 'ended');
  $gameHud.classList.toggle('hidden', state.phase !== 'playing');
  $cooldownIndicator.classList.toggle('hidden', state.phase !== 'playing');

  if (state.phase === 'waiting') {
    const waitingTitle: HTMLElement | null = document.getElementById('waiting-title');
    if (waitingTitle) {
      if (!state.nicknameSubmitted) {
        waitingTitle.textContent = '准备开始...';
      } else if (state.players.length > 1) {
        waitingTitle.textContent = '等待其他玩家确认昵称...';
      } else {
        waitingTitle.textContent = '即将开始...';
      }
    }
  }

  // Nickname setup only before the first round; never during countdown/playing/ended.
  const hideNick = state.phase === 'countdown' || state.phase === 'playing' || state.phase === 'ended';
  if ($nicknameSetupScreen && hideNick) $nicknameSetupScreen.classList.add('hidden');
  if ($nicknameInline) $nicknameInline.classList.add('hidden');
}

function displayNickname(p: { playerIndex: number; nickname: string }): string {
  if (
    state.pendingNickname
    && state.players.length === 1
    && state.players[0]?.playerIndex === p.playerIndex
  ) {
    return state.pendingNickname;
  }
  return p.nickname || 'P' + p.playerIndex;
}

function renderPlayerItems(
  container: HTMLElement,
  includeScore: boolean,
): void {
  container.textContent = '';
  for (const p of state.players) {
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

export function updateUI(force = false): void {
  if (state.connectionError && $waitingScreen) {
    const errorEl: Element | null = $waitingScreen.querySelector('.error-message');
    if (errorEl) errorEl.textContent = state.connectionError;
  }

  const phaseChanged = state.phase !== lastPhase;
  if (phaseChanged || force) {
    lastPhase = state.phase;
    setOverlayVisibility();
  }

  $hudScore.textContent = String(state.score);
  $hudPlayers.textContent = String(state.players.length);

  if (state.phase === 'ended') {
    $finalScore.textContent = String(state.score);
    const ek = endListKey();
    if (force || phaseChanged || ek !== lastEndListKey) {
      lastEndListKey = ek;
      const sorted = [...state.players].sort((a, b) => b.scoreContribution - a.scoreContribution);
      $endPlayerList.textContent = '';
      for (const p of sorted) {
        const color: string = PALETTE_COLORS[p.palette % PALETTE_COLORS.length]!;
        const div: HTMLDivElement = document.createElement('div');
        div.className = 'player-item';
        const dot: HTMLSpanElement = document.createElement('span');
        dot.className = 'player-dot';
        dot.style.background = color;
        const name: HTMLSpanElement = document.createElement('span');
        name.className = 'player-name';
        name.textContent = displayNickname(p);
        const score: HTMLSpanElement = document.createElement('span');
        score.className = 'player-score';
        score.textContent = String(p.scoreContribution);
        div.appendChild(dot);
        div.appendChild(name);
        div.appendChild(score);
        $endPlayerList.appendChild(div);
      }
    }
  }

  if (state.phase === 'ended' && state.restartVotes) {
    const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
    const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
    if ($restartProgress) {
      if (state.restartVotes.yes >= state.restartVotes.total && state.restartVotes.total > 0) {
        $restartProgress.textContent = '正在重启游戏...';
      } else {
        $restartProgress.textContent = `${state.restartVotes.yes}/${state.restartVotes.total} 玩家同意重启`;
      }
    }
    if (state.restartVotes.countdownMs <= 0 && $restartCountdown) {
      $restartCountdown.textContent = '';
    }
  }

  const pk = playerListKey();
  if (state.phase === 'playing' || state.phase === 'countdown') {
    if (force || phaseChanged || pk !== lastPlayerListKey) {
      lastPlayerListKey = pk;
      renderPlayerItems($hudPlayerList, true);
    } else {
      syncHudPlayerScores();
    }
  }

  if (state.phase === 'waiting') {
    if (force || phaseChanged || pk !== lastPlayerListKey) {
      lastPlayerListKey = pk;
      renderPlayerItems($playerListWaiting, false);
    }
  }
}

/** Lightweight HUD refresh on every snapshot — no DOM rebuild unless roster changed. */
export function updateScoresOnly(): void {
  $hudScore.textContent = String(state.score);
  $hudPlayers.textContent = String(state.players.length);

  const pk = playerListKey();
  if (state.phase === 'playing' || state.phase === 'countdown') {
    if (pk !== lastPlayerListKey) {
      lastPlayerListKey = pk;
      renderPlayerItems($hudPlayerList, true);
    } else {
      syncHudPlayerScores();
    }
  } else if (state.phase === 'waiting' && pk !== lastPlayerListKey) {
    lastPlayerListKey = pk;
    renderPlayerItems($playerListWaiting, false);
  }

  if (state.phase === 'ended') {
    $finalScore.textContent = String(state.score);
    const ek = endListKey();
    if (ek !== lastEndListKey) {
      lastEndListKey = ek;
      updateUI(true);
    }
  }
}
