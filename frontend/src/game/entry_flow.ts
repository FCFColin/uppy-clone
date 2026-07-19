import type { GamePhase } from './state.js';
import { dispatch, getState } from './state.js';
import { $canvas } from './renderer.js';
import { $lobbyCode, $hudCode } from './ui_common.js';
import { matchNewRoomCode } from './lobby_match.js';

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

// ─── Entry flow UI (merged from entry_flow_ui.ts) ────────────────────────

export type EntryStep = 'connecting' | 'nickname' | 'waiting' | 'handoff' | 'error';

export interface EntryFullScreenErrorOptions {
  showActions?: boolean;
  title?: string;
  midGameDisconnect?: boolean;
}

export interface EntryOverlayContext {
  entryStep: EntryStep;
  wsConnected: boolean;
  lobbyCode: string;
  phase: GamePhase;
  getWaitingTitleText: () => string;
}

const $loadingOverlay = document.getElementById('loading-overlay');
const $waitingTitle = document.getElementById('waiting-title');
const $loadingErrorPanel = document.getElementById('loading-error-panel');
const $loadingErrorText = document.getElementById('loading-error-text');
const $loadingErrorTitle = document.getElementById('loading-error-title');
const $loadingErrorActions = document.getElementById('loading-error-actions');
const $loadingSpinner = $loadingOverlay?.querySelector('.loading-spinner');
const $loadingText = $loadingOverlay?.querySelector('.loading-text');

let errorActionsBound = false;

export function setNicknameStatus(text: string): void {
  const el = document.getElementById('nickname-connect-status');
  if (el) el.textContent = text;
}

export function nicknameReadyStatus(lobbyCode: string, wsConnected: boolean): void {
  const code = lobbyCode || '-----';
  if (wsConnected) {
    setNicknameStatus(`服务器已连接 · 房间 ${code} · 点击「进入游戏」按钮加入`);
  } else {
    setNicknameStatus(`房间 ${code} 已就绪 · 设置昵称后进入`);
  }
}

export function setWaitingInlineError(text: string): void {
  const el = document.getElementById('waiting-connect-error');
  if (!el) return;
  el.textContent = text;
  el.classList.toggle('hidden', !text);
}

export function clearWaitingInlineError(): void {
  setWaitingInlineError('');
}

export function showLoadingOverlay(message = '正在连接房间…'): void {
  if (!$loadingOverlay) return;
  $loadingOverlay.classList.remove('hidden');
  $loadingOverlay.style.display = '';
  delete $loadingOverlay.dataset.error;
  $loadingSpinner?.classList.remove('hidden');
  $loadingText?.classList.remove('hidden');
  if ($loadingText) $loadingText.textContent = message;
  $loadingErrorPanel?.classList.add('hidden');
}

export function hideLoadingOverlay(): void {
  if (!$loadingOverlay) return;
  $loadingOverlay.style.pointerEvents = 'none';
  $loadingOverlay.classList.add('hidden');
}

export function updateWaitingStatusLine(ctx: EntryOverlayContext): void {
  if (!$waitingTitle) return;
  if (!ctx.wsConnected) {
    $waitingTitle.textContent = '已加入等待大厅 · 正在连接服务器…';
    return;
  }
  $waitingTitle.textContent = ctx.getWaitingTitleText();
}

function syncCanvasPointerEvents(entryStep: EntryStep, phase: GamePhase): void {
  $canvas.style.pointerEvents = entryStep === 'handoff' && phase === 'playing' ? 'auto' : 'none';
}

function isEntryStepLoading(entryStep: EntryStep): boolean {
  return entryStep === 'connecting' || entryStep === 'error';
}

function ensureEntryOverlayOnTop(entryStep: EntryStep): void {
  if ($loadingOverlay && !isEntryStepLoading(entryStep)) {
    $loadingOverlay.style.pointerEvents = 'none';
  }
  const inEntry = entryStep === 'nickname' || entryStep === 'waiting';
  document.getElementById('nickname-setup-screen')?.classList.toggle('entry-overlay-active', inEntry);
  document.getElementById('waiting-screen')?.classList.toggle('entry-overlay-active', inEntry);
}

export function syncEntryOverlays(ctx: EntryOverlayContext): void {
  const nickname = document.getElementById('nickname-setup-screen');
  const waiting = document.getElementById('waiting-screen');

  const showLoading = isEntryStepLoading(ctx.entryStep);
  const showNickname = ctx.entryStep === 'nickname';
  const showWaiting = ctx.entryStep === 'waiting';
  const isHandoff = ctx.entryStep === 'handoff';

  if ($loadingOverlay) {
    if (showLoading && ctx.entryStep === 'connecting') {
      showLoadingOverlay();
    } else if (!showLoading) {
      hideLoadingOverlay();
    }
  }

  nickname?.classList.toggle('hidden', !showNickname || isHandoff);
  waiting?.classList.toggle('hidden', !showWaiting || isHandoff);
  if (isHandoff) {
    nickname?.classList.remove('entry-overlay-active');
    waiting?.classList.remove('entry-overlay-active');
  }

  syncCanvasPointerEvents(ctx.entryStep, ctx.phase);
  ensureEntryOverlayOnTop(ctx.entryStep);

  if (showNickname) {
    nicknameReadyStatus(ctx.lobbyCode, ctx.wsConnected);
  }

  if (showWaiting) {
    updateWaitingStatusLine(ctx);
  }
}

export function setLobbyCodeDisplay(lobbyCode: string): void {
  $lobbyCode.textContent = lobbyCode;
  $hudCode.textContent = lobbyCode;
}

const ERROR_TITLES: Array<[string[], string]> = [
  [['已结束'], '房间已结束'],
  [['不存在'], '无法进入房间'],
  [['超时', '网络', '连接'], '连接失败'],
];

function errorTitleForMessage(message: string, midGameDisconnect?: boolean): string {
  if (midGameDisconnect) return '对局连接中断';
  for (const [keywords, title] of ERROR_TITLES) {
    if (keywords.some(k => message.includes(k))) return title;
  }
  return '无法进入房间';
}

export function renderEntryFullScreenError(message: string, options?: EntryFullScreenErrorOptions): void {
  if (!$loadingOverlay) return;
  $loadingOverlay.classList.remove('hidden');
  $loadingOverlay.dataset.error = 'true';
  $loadingOverlay.style.display = 'flex';
  $loadingOverlay.style.pointerEvents = 'auto';

  $loadingSpinner?.classList.add('hidden');
  $loadingText?.classList.add('hidden');

  if ($loadingErrorTitle) {
    $loadingErrorTitle.textContent = options?.title ?? errorTitleForMessage(message, options?.midGameDisconnect);
  }
  if ($loadingErrorText) $loadingErrorText.textContent = message;
  $loadingErrorPanel?.classList.remove('hidden');
  if ($loadingErrorActions) {
    $loadingErrorActions.classList.toggle('hidden', !options?.showActions);
  }

  bindEntryErrorPanelActions();
  document.getElementById('reconnect-banner')?.classList.add('hidden');
}

function bindEntryErrorPanelActions(): void {
  if (errorActionsBound) return;
  errorActionsBound = true;

  document.getElementById('loading-back-btn')?.addEventListener('click', () => {
    window.location.href = '/';
  });

  const matchBtn = document.getElementById('loading-match-btn');
  if (matchBtn) {
    matchBtn.addEventListener('click', () => {
      void (async () => {
        matchBtn.setAttribute('disabled', 'true');
        const code = await matchNewRoomCode();
        if (code) {
          window.location.href = `/play.html?code=${code}`;
          return;
        }
        matchBtn.removeAttribute('disabled');
        if ($loadingErrorText) {
          $loadingErrorText.textContent = '匹配失败，请稍后重试或返回大厅';
        }
      })();
    });
  }
}

export function renderStartCountdownTitle(remaining: number): void {
  if (!$waitingTitle) return;
  if (remaining <= 0) {
    $waitingTitle.textContent = '正在开始…';
    return;
  }
  $waitingTitle.textContent = `即将开始 · ${remaining}…`;
}

/** Test-only reset of DOM module state. */
export function resetEntryDomState(): void {
  errorActionsBound = false;
}

export function getWaitingTitleText(): string {
  if (getState().players.length > 1) {
    return '等待其他玩家确认昵称…';
  }
  return '即将开始…';
}
