import type { GamePhase } from '../shared/types.js';
import { state } from './state.js';
import {
  type EntryStep,
  type EntryFullScreenErrorOptions,
  type EntryOverlayContext,
  syncEntryOverlays,
  updateWaitingStatusLine,
  setNicknameStatus,
  setWaitingInlineError,
  clearWaitingInlineError,
  renderEntryFullScreenError,
  renderStartCountdownTitle,
  setLobbyCodeDisplay,
  nicknameReadyStatus,
  resetEntryDomForTest,
} from './entry_flow_dom.js';

export type { EntryStep, EntryFullScreenErrorOptions } from './entry_flow_dom.js';

let entryStep: EntryStep = 'connecting';
let lobbyPublished = false;
let wsConnected = false;
let entryUiBound = false;

const STEP_RANK: Record<EntryStep, number> = {
  connecting: 0,
  error: 0,
  nickname: 1,
  waiting: 2,
  handoff: 3,
};

function canAdvanceTo(next: EntryStep): boolean {
  return STEP_RANK[next] >= STEP_RANK[entryStep];
}

function overlayContext(): EntryOverlayContext {
  return {
    entryStep,
    wsConnected,
    getWaitingTitleText,
  };
}

function syncOverlays(): void {
  syncEntryOverlays(overlayContext());
}

/** Shared waiting-screen title during entry and post-handoff waiting phase. */
export function getWaitingTitleText(): string {
  if (state.players.length > 1) {
    return '等待其他玩家确认昵称…';
  }
  return '即将开始…';
}

/** Full-screen loading overlay error panel (connecting / error / mid-game disconnect). */
export function showEntryFullScreenError(message: string, options?: EntryFullScreenErrorOptions): void {
  applyEntryStep('error');
  renderEntryFullScreenError(message, options);
}

export function getEntryStep(): EntryStep {
  return entryStep;
}

export function isEntryHandoff(): boolean {
  return entryStep === 'handoff';
}

export function applyEntryStep(next: EntryStep): void {
  if (next === 'error') {
    entryStep = 'error';
    syncOverlays();
    return;
  }
  if (next === 'handoff') {
    entryStep = 'handoff';
    syncOverlays();
    return;
  }
  if (!canAdvanceTo(next)) return;
  entryStep = next;
  syncOverlays();
}

/** First lobby code resolved — idempotent after waiting/handoff. */
export function onLobbyCodeReady(lobbyCode: string): void {
  if (entryStep === 'waiting' || entryStep === 'handoff') return;
  if (lobbyPublished && entryStep !== 'connecting' && entryStep !== 'error') return;

  state.lobbyCode = lobbyCode;
  setLobbyCodeDisplay(lobbyCode);
  lobbyPublished = true;
  applyEntryStep('nickname');
}

/** User clicked「进入游戏」. */
export function onNicknameSubmit(): void {
  if (entryStep !== 'nickname') return;
  state.nicknameSubmitted = true;
  lobbyPublished = true;
  applyEntryStep('waiting');
  startStartCountdown();
}

let startCountdownTimer: ReturnType<typeof setInterval> | null = null;

/** Show a visible countdown on the waiting screen so the player has time to read room code & tips. */
function startStartCountdown(): void {
  clearStartCountdown();
  let remaining = 2;
  const tick = (): void => {
    renderStartCountdownTitle(remaining);
    if (remaining <= 0) {
      clearStartCountdown();
      return;
    }
    remaining--;
  };
  tick();
  startCountdownTimer = setInterval(tick, 1000);
}

export function clearStartCountdown(): void {
  if (startCountdownTimer !== null) {
    clearInterval(startCountdownTimer);
    startCountdownTimer = null;
  }
}

/** Bind nickname form submit — single entry point for enter-game UI. */
export function bindEntryUI(onSubmit: () => void): void {
  if (entryUiBound) return;
  entryUiBound = true;

  const form = document.getElementById('nickname-entry-form');
  form?.addEventListener('submit', (e: Event) => {
    e.preventDefault();
    if (entryStep !== 'nickname') return;
    onSubmit();
  });
}

/** WebSocket connected — update waiting status only, never regress step. */
export function onWebSocketOpen(): void {
  wsConnected = true;
  if (entryStep === 'waiting') {
    if (startCountdownTimer === null) {
      updateWaitingStatusLine(overlayContext());
    }
  } else if (entryStep === 'nickname') {
    nicknameReadyStatus(state.lobbyCode, wsConnected);
  }
}

export function onWebSocketClosed(): void {
  wsConnected = false;
  if (entryStep === 'waiting') {
    updateWaitingStatusLine(overlayContext());
  } else if (entryStep === 'nickname') {
    const code = state.lobbyCode || '-----';
    setNicknameStatus(`连接已断开 · 房间 ${code} · 仍可点击「进入游戏」（将自动重连）`);
  }
}

/** Enter handoff when server phase moves into active gameplay. */
export function tryEntryHandoff(phase: GamePhase): void {
  if (!state.nicknameSubmitted) return;
  if (phase === 'countdown' || phase === 'playing') {
    applyEntryStep('handoff');
  }
}

/** Inline or full-screen connection error depending on entry step. */
export function routeConnectionError(message: string, options?: EntryFullScreenErrorOptions): void {
  if (options?.showActions) {
    showEntryFullScreenError(message, options);
    return;
  }
  if (!options?.midGameDisconnect && (entryStep === 'nickname' || entryStep === 'waiting')) {
    if (entryStep === 'nickname') {
      setNicknameStatus(message);
    } else {
      setWaitingInlineError(message);
    }
    return;
  }
  showEntryFullScreenError(message, options);
}

export { clearWaitingInlineError };

export function initEntryFlow(): void {
  lobbyPublished = false;
  wsConnected = false;
  const code = new URLSearchParams(window.location.search).get('code')?.trim();
  if (code) {
    state.lobbyCode = code;
    setLobbyCodeDisplay(code);
    lobbyPublished = true;
    entryStep = 'nickname';
  } else {
    entryStep = 'connecting';
  }
  syncOverlays();
}

/** Test-only reset */
export function resetEntryFlowForTest(): void {
  clearStartCountdown();
  entryStep = 'connecting';
  lobbyPublished = false;
  wsConnected = false;
  entryUiBound = false;
  resetEntryDomForTest();
}
