import { state } from './state_types.js';
import { $canvas } from './renderer_canvas.js';
import { $lobbyCode, $hudCode } from './ui_elements.js';
import { matchNewRoomCode } from './room_validate.js';

export type EntryStep = 'connecting' | 'nickname' | 'waiting' | 'handoff' | 'error';

const $loadingOverlay = document.getElementById('loading-overlay');
const $waitingTitle = document.getElementById('waiting-title');
const $loadingErrorPanel = document.getElementById('loading-error-panel');
const $loadingErrorText = document.getElementById('loading-error-text');
const $loadingErrorTitle = document.getElementById('loading-error-title');
const $loadingErrorActions = document.getElementById('loading-error-actions');
const $loadingSpinner = $loadingOverlay?.querySelector('.loading-spinner');
const $loadingText = $loadingOverlay?.querySelector('.loading-text');

let errorActionsBound = false;

export interface EntryFullScreenErrorOptions {
  showActions?: boolean;
  title?: string;
  midGameDisconnect?: boolean;
}

export interface EntryOverlayContext {
  entryStep: EntryStep;
  wsConnected: boolean;
  getWaitingTitleText: () => string;
}

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
  if (text) {
    el.textContent = text;
    el.classList.remove('hidden');
  } else {
    el.textContent = '';
    el.classList.add('hidden');
  }
}

export function clearWaitingInlineError(): void {
  setWaitingInlineError('');
}

export function showLoadingOverlay(message = '正在连接房间…'): void {
  if (!$loadingOverlay) return;
  $loadingOverlay.classList.remove('hidden');
  $loadingOverlay.style.display = 'flex';
  delete $loadingOverlay.dataset.error;
  $loadingSpinner?.classList.remove('hidden');
  $loadingText?.classList.remove('hidden');
  if ($loadingText) $loadingText.textContent = message;
  $loadingErrorPanel?.classList.add('hidden');
}

export function hideLoadingOverlay(): void {
  if (!$loadingOverlay) return;
  $loadingOverlay.style.display = 'none';
  $loadingOverlay.style.pointerEvents = 'none';
  $loadingOverlay.classList.add('hidden');
}

function syncCanvasPointerEvents(entryStep: EntryStep): void {
  $canvas.style.pointerEvents = entryStep === 'handoff' && state.phase === 'playing' ? 'auto' : 'none';
}

function ensureEntryOverlayOnTop(entryStep: EntryStep): void {
  const showLoading = entryStep === 'connecting' || entryStep === 'error';
  if ($loadingOverlay && !showLoading) {
    $loadingOverlay.style.pointerEvents = 'none';
  }
  const inEntry = entryStep === 'nickname' || entryStep === 'waiting';
  document.getElementById('nickname-setup-screen')?.classList.toggle('entry-overlay-active', inEntry);
  document.getElementById('waiting-screen')?.classList.toggle('entry-overlay-active', inEntry);
}

export function updateWaitingStatusLine(ctx: EntryOverlayContext): void {
  if (!$waitingTitle) return;
  if (!ctx.wsConnected) {
    $waitingTitle.textContent = '已加入等待大厅 · 正在连接服务器…';
    return;
  }
  $waitingTitle.textContent = ctx.getWaitingTitleText();
}

export function syncEntryOverlays(ctx: EntryOverlayContext): void {
  const nickname = document.getElementById('nickname-setup-screen');
  const waiting = document.getElementById('waiting-screen');

  const showLoading = ctx.entryStep === 'connecting' || ctx.entryStep === 'error';
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

  syncCanvasPointerEvents(ctx.entryStep);
  ensureEntryOverlayOnTop(ctx.entryStep);

  if (showNickname) {
    nicknameReadyStatus(state.lobbyCode, ctx.wsConnected);
  }

  if (showWaiting) {
    updateWaitingStatusLine(ctx);
  }
}

export function setLobbyCodeDisplay(lobbyCode: string): void {
  $lobbyCode.textContent = lobbyCode;
  $hudCode.textContent = lobbyCode;
}

function errorTitleForMessage(message: string, midGameDisconnect?: boolean): string {
  if (midGameDisconnect) return '对局连接中断';
  if (message.includes('已结束')) return '房间已结束';
  if (message.includes('不存在')) return '无法进入房间';
  if (message.includes('超时') || message.includes('网络') || message.includes('连接')) return '连接失败';
  return '无法进入房间';
}

/** Full-screen loading overlay error panel (connecting / error / mid-game disconnect). */
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

export function bindEntryErrorPanelActions(): void {
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

export function resetEntryDomForTest(): void {
  errorActionsBound = false;
}
