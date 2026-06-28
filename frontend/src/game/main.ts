import { encodeSetNickname } from './message_codec.js';
import { state, seenSeqs, getInterpState } from './state.js';
import { normalizeAuthHost } from '../shared/session.js';
import { resumeAudioContext } from '../shared/audio.js';
import { showToast } from '../shared/toast.js';
import { resizeCanvas, gameLoop, setRenderActive, renderOnce, $canvas } from './renderer.js';
import {
  updateUI, generateRandomNickname, copyCode, refreshLayout,
  showFallbackErrorScreen,
  $copyCodeBtn, $hudCopyBtn,
  $setupNicknameInput,
} from './ui.js';
import {
  connectWebSocket, sendOrQueue,
  stopHeartbeat, getWs, showConnectionError,
} from './websocket.js';
import { handleTap, requestRestart, tapAtBalloonCenter } from './input.js';
import { initWaitingTips } from './waiting_tips.js';
import { bindReconnectRetry } from './connection_ui.js';
import {
  initEntryFlow, bindEntryUI, onNicknameSubmit, onWebSocketOpen, getEntryStep,
} from './entry_flow.js';

normalizeAuthHost();

// Save current game URL so the leaderboard page can offer a "返回游戏" button.
localStorage.setItem('uppy-game-url', window.location.href);

if (import.meta.env.DEV) {
  window.state = state;
  window.__gamePhase = state.phase;
  window.__seenSeqs = seenSeqs;
  window.__interp = getInterpState();
}

window.requestRestart = requestRestart;
window.generateRandomNickname = generateRandomNickname;

function submitSetupNickname(): Promise<void> {
  const input: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;
  let nickname: string = input ? input.value.trim() : '';
  if (!nickname) {
    nickname = generateRandomNickname();
  }
  localStorage.setItem('uppy-nickname', nickname);
  state.pendingNickname = nickname;
  onNicknameSubmit();

  const msg: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(msg);

  updateUI(true);
  showToast(`欢迎，${nickname}！`);
  return Promise.resolve();
}
window.submitSetupNickname = submitSetupNickname;

const savedNickname: string | null = localStorage.getItem('uppy-nickname');
if (savedNickname && $setupNicknameInput) {
  $setupNicknameInput.value = savedNickname;
} else if ($setupNicknameInput) {
  $setupNicknameInput.value = generateRandomNickname();
}

initEntryFlow();
bindEntryUI(() => {
  submitSetupNickname();
});

initWaitingTips();
bindReconnectRetry(() => {
  void connectWebSocket();
});

connectWebSocket();
requestAnimationFrame(gameLoop);

setTimeout(() => {
  if (getEntryStep() !== 'connecting') return;
  showConnectionError('连接超时，请检查网络或稍后重试', { showActions: true });
}, 8000);

window.addEventListener('game-ws-open', () => {
  onWebSocketOpen();
  updateUI(true);
});

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

window.addEventListener('resize', handleResize);
window.addEventListener('orientationchange', handleResize);
resizeCanvas();
renderOnce();

if ($copyCodeBtn) $copyCodeBtn.addEventListener('click', () => { void copyCode(); });
if ($hudCopyBtn) $hudCopyBtn.addEventListener('click', () => { void copyCode(); });

document.getElementById('random-nickname-btn')?.addEventListener('click', () => {
  if ($setupNicknameInput) $setupNicknameInput.value = generateRandomNickname();
});
document.getElementById('restart-btn')?.addEventListener('click', requestRestart);

document.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === ' ' || e.key === 'Enter') {
    if (state.phase === 'playing' && document.activeElement?.tagName !== 'INPUT') {
      e.preventDefault();
      tapAtBalloonCenter();
    }
  }
});

document.addEventListener('visibilitychange', () => {
  setRenderActive(!document.hidden);
  if (!document.hidden) requestAnimationFrame(gameLoop);
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
  state.connectionError = '网络已断开';
});

window.addEventListener('beforeunload', () => {
  stopHeartbeat();
  const ws: WebSocket | null = getWs();
  if (ws) {
    ws.onclose = null;
    ws.close(1000, 'page unload');
  }
});
