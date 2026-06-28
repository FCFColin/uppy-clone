import type { GamePhase } from '../shared/types.js';
import { state } from './state.js';
import { $canvas } from './renderer_canvas.js';
import { $lobbyCode, $hudCode } from './ui_elements.js';
import { matchNewRoomCode } from './room_validate.js';

export type EntryStep = 'connecting' | 'nickname' | 'waiting' | 'handoff' | 'error';

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

function setNicknameStatus(text: string): void {
  const el = document.getElementById('nickname-connect-status');
  if (el) el.textContent = text;
}

function nicknameReadyStatus(): void {
  const code = state.lobbyCode || '-----';
  if (wsConnected) {
    setNicknameStatus(`服务器已连接 · 房间 ${code} · 点击「进入游戏」按钮加入`);
  } else {
    setNicknameStatus(`房间 ${code} 已就绪 · 设置昵称后进入`);
  }
}

function setWaitingInlineError(text: string): void {
  const el = document.getElementById('waiting-connect-error');
  if (!el) return;
  if (text) {
    el.textContent = text;
    el.classList.remove('hidden');
  } else {
    el.textContent = '';
    el.classList.add('hidden');
  }
}

function showLoadingOverlay(message = '正在连接房间…'): void {
  const overlay = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.classList.remove('hidden');
  overlay.style.display = 'flex';
  delete overlay.dataset.error;
  overlay.querySelector('.loading-spinner')?.classList.remove('hidden');
  overlay.querySelector('.loading-text')?.classList.remove('hidden');
  const loadingText = overlay.querySelector('.loading-text');
  if (loadingText) loadingText.textContent = message;
  document.getElementById('loading-error-panel')?.classList.add('hidden');
}

function hideLoadingOverlay(): void {
  const overlay = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.style.display = 'none';
  overlay.style.pointerEvents = 'none';
  overlay.classList.add('hidden');
}

function syncCanvasPointerEvents(): void {
  $canvas.style.pointerEvents = entryStep === 'handoff' && state.phase === 'playing' ? 'auto' : 'none';
}

function ensureEntryOverlayOnTop(): void {
  const loading = document.getElementById('loading-overlay');
  const showLoading = entryStep === 'connecting' || entryStep === 'error';
  if (loading && !showLoading) {
    loading.style.pointerEvents = 'none';
  }
  const inEntry = entryStep === 'nickname' || entryStep === 'waiting';
  document.getElementById('nickname-setup-screen')?.classList.toggle('entry-overlay-active', inEntry);
  document.getElementById('waiting-screen')?.classList.toggle('entry-overlay-active', inEntry);
}

function syncEntryOverlays(): void {
  const loading = document.getElementById('loading-overlay');
  const nickname = document.getElementById('nickname-setup-screen');
  const waiting = document.getElementById('waiting-screen');

  const showLoading = entryStep === 'connecting' || entryStep === 'error';
  const showNickname = entryStep === 'nickname';
  const showWaiting = entryStep === 'waiting';
  const isHandoff = entryStep === 'handoff';

  if (loading) {
    if (showLoading && entryStep === 'connecting') {
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

  syncCanvasPointerEvents();
  ensureEntryOverlayOnTop();

  if (showNickname) {
    nicknameReadyStatus();
  }

  if (showWaiting) {
    updateWaitingStatusLine();
  }
}

function updateWaitingStatusLine(): void {
  const title = document.getElementById('waiting-title');
  if (!title) return;
  if (!wsConnected) {
    title.textContent = '已加入等待大厅 · 正在连接服务器…';
    return;
  }
  title.textContent = getWaitingTitleText();
}

/** Shared waiting-screen title during entry and post-handoff waiting phase. */
export function getWaitingTitleText(): string {
  if (state.players.length > 1) {
    return '等待其他玩家确认昵称…';
  }
  return '即将开始…';
}

let errorActionsBound = false;

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
        const errorText = document.getElementById('loading-error-text');
        if (errorText) {
          errorText.textContent = '匹配失败，请稍后重试或返回大厅';
        }
      })();
    });
  }
}

export interface EntryFullScreenErrorOptions {
  showActions?: boolean;
  title?: string;
  midGameDisconnect?: boolean;
}

function errorTitleForMessage(message: string, midGameDisconnect?: boolean): string {
  if (midGameDisconnect) return '对局连接中断';
  if (message.includes('已结束')) return '房间已结束';
  if (message.includes('不存在')) return '无法进入房间';
  if (message.includes('超时') || message.includes('网络') || message.includes('连接')) return '连接失败';
  return '无法进入房间';
}

/** Full-screen loading overlay error panel (connecting / error / mid-game disconnect). */
export function showEntryFullScreenError(message: string, options?: EntryFullScreenErrorOptions): void {
  applyEntryStep('error');
  const overlay = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.classList.remove('hidden');
  overlay.dataset.error = 'true';
  overlay.style.display = 'flex';
  // Must clear inline pointer-events:'none' set by hideLoadingOverlay,
  // otherwise the CSS rule [data-error='true']{pointer-events:auto} is overridden.
  overlay.style.pointerEvents = 'auto';

  overlay.querySelector('.loading-spinner')?.classList.add('hidden');
  overlay.querySelector('.loading-text')?.classList.add('hidden');

  const errorTitle = document.getElementById('loading-error-title');
  const errorText = document.getElementById('loading-error-text');
  const errorPanel = document.getElementById('loading-error-panel');
  const actions = document.getElementById('loading-error-actions');

  if (errorTitle) {
    errorTitle.textContent = options?.title ?? errorTitleForMessage(message, options?.midGameDisconnect);
  }
  if (errorText) errorText.textContent = message;
  errorPanel?.classList.remove('hidden');
  if (actions) {
    actions.classList.toggle('hidden', !options?.showActions);
  }

  bindEntryErrorPanelActions();
  document.getElementById('reconnect-banner')?.classList.add('hidden');
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
    syncEntryOverlays();
    return;
  }
  if (next === 'handoff') {
    entryStep = 'handoff';
    syncEntryOverlays();
    return;
  }
  if (!canAdvanceTo(next)) return;
  entryStep = next;
  syncEntryOverlays();
}

/** First lobby code resolved — idempotent after waiting/handoff. */
export function onLobbyCodeReady(lobbyCode: string): void {
  if (entryStep === 'waiting' || entryStep === 'handoff') return;
  if (lobbyPublished && entryStep !== 'connecting' && entryStep !== 'error') return;

  state.lobbyCode = lobbyCode;
  $lobbyCode.textContent = lobbyCode;
  $hudCode.textContent = lobbyCode;
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
    const title = document.getElementById('waiting-title');
    if (!title) return;
    if (remaining <= 0) {
      title.textContent = '正在开始…';
      clearStartCountdown();
      return;
    }
    title.textContent = `即将开始 · ${remaining}…`;
    remaining--;
  };
  tick();
  startCountdownTimer = setInterval(tick, 1000);
}

function clearStartCountdown(): void {
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
    // 不清除倒计时：倒计时是纯客户端 UI，与服务器 startDelay 并行运行。
    // 仅在倒计时已结束时更新状态行。
    if (startCountdownTimer === null) {
      updateWaitingStatusLine();
    }
  } else if (entryStep === 'nickname') {
    nicknameReadyStatus();
  }
}

export function onWebSocketClosed(): void {
  wsConnected = false;
  if (entryStep === 'waiting') {
    updateWaitingStatusLine();
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

/** @deprecated Use routeConnectionError */
export function onConnectError(message: string, options?: EntryFullScreenErrorOptions): void {
  routeConnectionError(message, options);
}

/** Test-only: clear the start countdown timer. */
export function clearStartCountdownForTest(): void {
  clearStartCountdown();
}

export function clearWaitingInlineError(): void {
  setWaitingInlineError('');
}

export function initEntryFlow(): void {
  lobbyPublished = false;
  wsConnected = false;
  const code = new URLSearchParams(window.location.search).get('code')?.trim();
  if (code) {
    state.lobbyCode = code;
    $lobbyCode.textContent = code;
    $hudCode.textContent = code;
    lobbyPublished = true;
    entryStep = 'nickname';
  } else {
    entryStep = 'connecting';
  }
  syncEntryOverlays();
}

/** Test-only reset */
export function resetEntryFlowForTest(): void {
  clearStartCountdown();
  entryStep = 'connecting';
  lobbyPublished = false;
  wsConnected = false;
  entryUiBound = false;
  errorActionsBound = false;
}
