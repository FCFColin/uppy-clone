import type { GamePhase } from './state.js';
import { dispatch, getState } from './state.js';
import type { EntryStep, EntryOverlayContext, EntryFullScreenErrorOptions } from './entry_flow_ui.js';
import {
  syncEntryOverlays, setLobbyCodeDisplay, renderStartCountdownTitle,
  renderEntryFullScreenError, setNicknameStatus, nicknameReadyStatus,
  setWaitingInlineError, updateWaitingStatusLine, resetEntryDomState,
  getWaitingTitleText,
} from './entry_flow_ui.js';

export type { EntryOverlayContext, EntryFullScreenErrorOptions };

let entryUiBound = false;

const STEP_RANK: Record<EntryStep, number> = {
  connecting: 0,
  error: 0,
  nickname: 1,
  waiting: 2,
  handoff: 3,
};

function canAdvanceTo(next: EntryStep): boolean {
  return STEP_RANK[next] >= STEP_RANK[getState().entryStep];
}

function overlayContext(): EntryOverlayContext {
  const s = getState();
  return {
    entryStep: s.entryStep,
    wsConnected: s.wsConnected,
    lobbyCode: s.lobbyCode,
    phase: s.phase,
    getWaitingTitleText,
  };
}

function syncOverlays(): void {
  syncEntryOverlays(overlayContext());
}

/** Full-screen loading overlay error panel (connecting / error / mid-game disconnect). */
export function showEntryFullScreenError(message: string, options?: EntryFullScreenErrorOptions): void {
  applyEntryStep('error');
  renderEntryFullScreenError(message, options);
}

export function getEntryStep(): EntryStep {
  return getState().entryStep;
}

export function isEntryHandoff(): boolean {
  return getState().entryStep === 'handoff';
}

export function applyEntryStep(next: EntryStep): void {
  if (canAdvanceTo(next)) {
    dispatch({ type: 'SET_STATE', partial: { entryStep: next } });
    syncOverlays();
  }
}

/** First lobby code resolved — idempotent after waiting/handoff. */
export function onLobbyCodeReady(lobbyCode: string): void {
  const s = getState();
  if (s.entryStep === 'waiting' || s.entryStep === 'handoff') return;
  if (s.lobbyPublished && s.entryStep !== 'connecting' && s.entryStep !== 'error') return;

  dispatch({ type: 'SET_STATE', partial: { lobbyCode, lobbyPublished: true } });
  setLobbyCodeDisplay(lobbyCode);
  applyEntryStep('nickname');
}

/** User clicked「进入游戏」. */
export function onNicknameSubmit(): void {
  if (getState().entryStep !== 'nickname') return;
  dispatch({ type: 'SET_STATE', partial: { nicknameSubmitted: true, lobbyPublished: true } });
  applyEntryStep('waiting');
  startStartCountdown();
}

let startCountdownTimer: ReturnType<typeof setInterval> | null = null;

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
    if (getState().entryStep !== 'nickname') return;
    onSubmit();
  });
}

/** WebSocket connected — update waiting status only, never regress step. */
export function onWebSocketOpen(): void {
  dispatch({ type: 'SET_STATE', partial: { wsConnected: true } });
  const s = getState();
  if (s.entryStep === 'waiting') {
    if (startCountdownTimer === null) {
      updateWaitingStatusLine(overlayContext());
    }
  } else if (s.entryStep === 'nickname') {
    nicknameReadyStatus(s.lobbyCode, s.wsConnected);
  }
}

export function onWebSocketClosed(): void {
  dispatch({ type: 'SET_STATE', partial: { wsConnected: false } });
  const s = getState();
  if (s.entryStep === 'waiting') {
    updateWaitingStatusLine(overlayContext());
  } else if (s.entryStep === 'nickname') {
    const code = s.lobbyCode || '-----';
    setNicknameStatus(`连接已断开 · 房间 ${code} · 仍可点击「进入游戏」（将自动重连）`);
  }
}

/** Enter handoff when server phase moves into active gameplay. */
export function tryEntryHandoff(phase: GamePhase): void {
  if (!getState().nicknameSubmitted) return;
  if (phase === 'countdown' || phase === 'playing') {
    applyEntryStep('handoff');
  }
}

/** Inline or full-screen connection error depending on entry step. */
export function routeConnectionError(message: string, options?: EntryFullScreenErrorOptions): void {
  const showFull = options?.showActions || options?.midGameDisconnect ||
    (getState().entryStep !== 'nickname' && getState().entryStep !== 'waiting');
  if (showFull) {
    showEntryFullScreenError(message, options);
    return;
  }
  if (getState().entryStep === 'nickname') {
    setNicknameStatus(message);
  } else {
    setWaitingInlineError(message);
  }
}

export function initEntryFlow(): void {
  const code = new URLSearchParams(window.location.search).get('code')?.trim();
  if (code) {
    dispatch({ type: 'SET_STATE', partial: { lobbyCode: code, lobbyPublished: true, wsConnected: false, entryStep: 'nickname' } });
    setLobbyCodeDisplay(code);
  } else {
    dispatch({ type: 'SET_STATE', partial: { lobbyPublished: false, wsConnected: false, entryStep: 'connecting' } });
  }
  syncOverlays();
}

/** Test-only reset */
export function resetEntryFlowForTest(): void {
  clearStartCountdown();
  dispatch({ type: 'SET_STATE', partial: { entryStep: 'connecting', lobbyPublished: false, wsConnected: false } });
  entryUiBound = false;
  resetEntryDomState();
}

/**
 * Production reset of entry-flow module-level state.
 * Called by resetClientState() — store fields (lobbyPublished, wsConnected)
 * are reset by RESET_ALL; this handles remaining module-local state.
 */
export function resetEntryFlowState(): void {
  clearStartCountdown();
  entryUiBound = false;
  resetEntryDomState();
}
