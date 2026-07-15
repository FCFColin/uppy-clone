import { resumeAudioContext } from '../shared/ui/audio.js';
import { handleTap, requestRestart, tapAtBalloonCenter } from './input.js';
import { getState, dispatch } from './store.js';
import { resizeCanvas, startGameLoop, setRenderActive, renderOnce, $canvas } from './renderer.js';
import { generateRandomNickname, copyCode, refreshLayout, showFallbackErrorScreen } from './ui_utils.js';
import { $copyCodeBtn, $hudCopyBtn, $setupNicknameInput } from './ui_elements.js';
import { connectWebSocket } from './ws_connect.js';
import { stopHeartbeat, getWs } from './ws_connection.js';
import { registerResetFn } from './reset_registry.js';

let lastTapTime = 0;
function handleTapWithDedup(clientX: number, clientY: number): void {
  const now = Date.now();
  if (now - lastTapTime < 200) return;
  lastTapTime = now;
  handleTap(clientX, clientY);
}

let resizeTimer: ReturnType<typeof setTimeout> | null = null;
function handleResize(): void {
  if (resizeTimer !== null) clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    resizeCanvas();
    refreshLayout();
    renderOnce();
    resizeTimer = null;
  }, 100);
}

export function bindWindowEvents(): void {
  $canvas.addEventListener('click', () => resumeAudioContext());
  $canvas.addEventListener('touchstart', () => resumeAudioContext(), { passive: true });

  $canvas.addEventListener('click', (e: MouseEvent) => {
    if ('ontouchstart' in window) return;
    handleTapWithDedup(e.clientX, e.clientY);
  });
  $canvas.addEventListener('touchstart', (e: TouchEvent) => {
    e.preventDefault();
    const touch: Touch = e.touches[0]!;
    handleTapWithDedup(touch.clientX, touch.clientY);
  }, { passive: false });

  window.addEventListener('resize', handleResize);
  window.addEventListener('orientationchange', handleResize);

  if ($copyCodeBtn) $copyCodeBtn.addEventListener('click', () => { void copyCode(); });
  if ($hudCopyBtn) $hudCopyBtn.addEventListener('click', () => { void copyCode(); });

  document.getElementById('random-nickname-btn')?.addEventListener('click', () => {
    if ($setupNicknameInput) $setupNicknameInput.value = generateRandomNickname();
  });
  document.getElementById('restart-btn')?.addEventListener('click', requestRestart);

  document.addEventListener('keydown', (e: KeyboardEvent) => {
    if (e.key === ' ' || e.key === 'Enter') {
      if (getState().phase === 'playing' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
        e.preventDefault();
        tapAtBalloonCenter();
      }
    }
  });

  document.addEventListener('visibilitychange', () => {
    setRenderActive(!document.hidden);
    if (!document.hidden) startGameLoop();
  });

  window.addEventListener('error', (e: ErrorEvent) => {
    console.error('[UNCAUGHT]', e.error);
    showFallbackErrorScreen(e.error?.message ?? '未知错误');
  });

  window.addEventListener('unhandledrejection', (e: PromiseRejectionEvent) => {
    console.error('Unhandled promise rejection:', e.reason);
    showFallbackErrorScreen(String(e.reason ?? '未知错误'));
  });

  window.addEventListener('online', () => {
    if (getWs()?.readyState !== WebSocket.OPEN) void connectWebSocket();
  });
  window.addEventListener('offline', () => {
    dispatch({ type: 'SET_STATE', partial: { connectionError: '网络已断开' } });
  });

  window.addEventListener('beforeunload', () => {
    stopHeartbeat();
    const ws: WebSocket | null = getWs();
    if (ws) {
      ws.onclose = null;
      ws.close(1000, 'page unload');
    }
  });
}

/** Reset window_events module-level state for a new game session. */
export function resetWindowEventState(): void {
  if (resizeTimer !== null) {
    clearTimeout(resizeTimer);
    resizeTimer = null;
  }
  lastTapTime = 0;
}

registerResetFn(resetWindowEventState);
