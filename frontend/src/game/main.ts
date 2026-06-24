import { textEncoder, CLIENT_MSG } from './constants.js';
import { encodeSetNickname } from './protocol.js';
import { state, seenSeqs, getInterpState } from './state.js';
import { resizeCanvas, gameLoop, setRenderActive, $canvas } from './renderer.js';
import {
  updateUI, generateRandomNickname, copyCode, checkOrientation,
  showFallbackErrorScreen,
  $copyCodeBtn, $hudCopyBtn,
  $nicknameInline, $nicknameInput, $nicknameBtn,
  $nicknameSetupScreen, $setupNicknameInput,
} from './ui.js';
import {
  connectWebSocket, sendOrQueue, waitForWebSocket,
  stopHeartbeat, getWs, showConnectionError,
} from './websocket.js';
import { handleTap, requestRestart } from './input.js';

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

  const nickBytes: Uint8Array = textEncoder.encode(nickname);
  const buf: ArrayBuffer = new ArrayBuffer(1 + 1 + nickBytes.length);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.SET_NICKNAME);
  dv.setUint8(1, nickBytes.length);
  new Uint8Array(buf, 2).set(nickBytes);
  sendOrQueue(buf);
}

async function submitSetupNickname(): Promise<void> {
  const input: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;
  let nickname: string = input ? input.value.trim() : '';
  if (!nickname) {
    nickname = generateRandomNickname();
  }
  localStorage.setItem('uppy-nickname', nickname);
  const setupScreen: HTMLElement | null = document.getElementById('nickname-setup-screen');
  if (setupScreen) setupScreen.classList.add('hidden');

  if (!getWs() || getWs()!.readyState !== WebSocket.OPEN) {
    await waitForWebSocket(5000);
  }
  const msg: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(msg);
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

$canvas.addEventListener('click', (e: MouseEvent) => handleTap(e.clientX, e.clientY));
$canvas.addEventListener('touchstart', (e: TouchEvent) => {
  e.preventDefault();
  const touch: Touch = e.touches[0]!;
  handleTap(touch.clientX, touch.clientY);
}, { passive: false });

if ($copyCodeBtn) $copyCodeBtn.addEventListener('click', copyCode);
if ($hudCopyBtn) $hudCopyBtn.addEventListener('click', copyCode);

// 按钮：随机昵称、进入游戏、重新开始（原 inline onclick，改为 addEventListener）
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

window.addEventListener('resize', resizeCanvas);
resizeCanvas();

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
  if (overlay && overlay.style.display !== 'none') {
    showConnectionError('加载超时，请检查网络或稍后重试');
  }
}, 8000);
