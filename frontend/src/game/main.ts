import { encodeSetNickname } from './protocol.js';
import { state, seenSeqs, getInterpState } from './state.js';
import { normalizeAuthHost } from '../shared/session.js';
import { resizeCanvas, gameLoop, setRenderActive, renderOnce, $canvas } from './renderer.js';
import {
  updateUI, generateRandomNickname, copyCode, checkOrientation,
  showFallbackErrorScreen,
  $copyCodeBtn, $hudCopyBtn,
  $nicknameInline, $nicknameInput, $nicknameBtn,
  $nicknameSetupScreen, $setupNicknameInput,
} from './ui.js';
import {
  connectWebSocket, sendOrQueue, waitForWebSocket,
  stopHeartbeat, getWs, getWsEverOpened, showConnectionError,
} from './websocket.js';
import { handleTap, requestRestart } from './input.js';

normalizeAuthHost();

window.state = state;
window.__gamePhase = state.phase;
window.requestRestart = requestRestart;
window.generateRandomNickname = generateRandomNickname;
window.__seenSeqs = seenSeqs;
window.__interp = getInterpState();

function submitNickname(): void {
  const nickname: string = $nicknameInput.value.trim() || ('Player ' + Math.floor(Math.random() * 999));
  localStorage.setItem('uppy-nickname', nickname);
  if ($nicknameInline) $nicknameInline.classList.add('hidden');

  const nickBytes: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(nickBytes);
}

async function submitSetupNickname(): Promise<void> {
  const input: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;
  let nickname: string = input ? input.value.trim() : '';
  if (!nickname) {
    nickname = generateRandomNickname();
  }
  localStorage.setItem('uppy-nickname', nickname);
  state.nicknameSubmitted = true;
  state.pendingNickname = nickname;
  const setupScreen: HTMLElement | null = document.getElementById('nickname-setup-screen');
  if (setupScreen) setupScreen.classList.add('hidden');

  if (!getWs() || getWs()!.readyState !== WebSocket.OPEN) {
    await waitForWebSocket(5000);
  }
  const msg: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(msg);
  updateUI(true);

  if (state.phase === 'waiting') {
    const waitingScreen: HTMLElement | null = document.getElementById('waiting-screen');
    if (waitingScreen) waitingScreen.classList.remove('hidden');
    updateUI(true);
  }
}
window.submitSetupNickname = submitSetupNickname;

const savedNickname: string | null = localStorage.getItem('uppy-nickname');
if (savedNickname && $setupNicknameInput) {
  $setupNicknameInput.value = savedNickname;
} else if ($setupNicknameInput) {
  $setupNicknameInput.value = generateRandomNickname();
}
if ($nicknameSetupScreen) $nicknameSetupScreen.classList.remove('hidden');

$nicknameBtn.addEventListener('click', submitNickname);
$nicknameInput.addEventListener('keydown', (e: KeyboardEvent) => { if (e.key === 'Enter') submitNickname(); });

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
    renderOnce();
    resizeTimer = null;
  }, 100);
}

window.addEventListener('resize', handleResize);
resizeCanvas();
renderOnce();

if ($copyCodeBtn) $copyCodeBtn.addEventListener('click', copyCode);
if ($hudCopyBtn) $hudCopyBtn.addEventListener('click', copyCode);

const $randomNicknameBtn: HTMLElement | null = document.getElementById('random-nickname-btn');
if ($randomNicknameBtn) {
  $randomNicknameBtn.addEventListener('click', () => {
    if ($setupNicknameInput) $setupNicknameInput.value = generateRandomNickname();
  });
}
const $enterGameBtn: HTMLElement | null = document.getElementById('enter-game-btn');
if ($enterGameBtn) {
  $enterGameBtn.addEventListener('click', () => { submitSetupNickname(); });
}
const $restartBtn: HTMLElement | null = document.getElementById('restart-btn');
if ($restartBtn) {
  $restartBtn.addEventListener('click', requestRestart);
}

window.addEventListener('resize', checkOrientation);
window.addEventListener('orientationchange', checkOrientation);

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
});

window.addEventListener('beforeunload', () => {
  stopHeartbeat();
  const ws: WebSocket | null = getWs();
  if (ws) {
    ws.onclose = null;
    ws.close(1000, 'page unload');
  }
});

connectWebSocket();
checkOrientation();
requestAnimationFrame(gameLoop);

setTimeout(() => {
  const overlay: HTMLElement | null = document.getElementById('loading-overlay');
  if (overlay && overlay.style.display !== 'none' && !overlay.dataset.error && !getWsEverOpened()) {
    showConnectionError('加载超时，请检查网络或稍后重试');
  }
}, 8000);
