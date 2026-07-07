import { encodeSetNickname } from './message_codec.js';
import { dispatch } from './store.js';
import { normalizeAuthHost } from '../shared/network/session.js';
import { showToast } from '../shared/ui/toast.js';
import { resizeCanvas, gameLoop, renderOnce } from './renderer.js';
import {
  updateUI, generateRandomNickname,
  $setupNicknameInput,
} from './ui.js';
import { connectWebSocket, showConnectionError } from './ws_connect.js';
import { invalidateNicknameCache } from './ui_update.js';
import { sendOrQueue } from './ws_connection.js';
import { initWaitingTips } from './waiting_tips.js';
import { bindReconnectRetry } from './connection_ui.js';
import {
  initEntryFlow, bindEntryUI, onNicknameSubmit, onWebSocketOpen, getEntryStep,
} from './entry_flow.js';

function submitSetupNickname(): void {
  const input: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;
  let nickname: string = input ? input.value.trim() : '';
  if (!nickname) {
    nickname = generateRandomNickname();
  }
  localStorage.setItem('uppy-nickname', nickname);
  dispatch({ type: 'SET_STATE', partial: { pendingNickname: nickname } });
  invalidateNicknameCache();
  onNicknameSubmit();

  const msg: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(msg);

  updateUI({ force: true });
  showToast(`欢迎，${nickname}！`);
}

function initNickname(): void {
  const savedNickname: string | null = localStorage.getItem('uppy-nickname');
  if (savedNickname && $setupNicknameInput) {
    $setupNicknameInput.value = savedNickname;
  } else if ($setupNicknameInput) {
    $setupNicknameInput.value = generateRandomNickname();
  }
}

function initConnection(): void {
  connectWebSocket();
  setTimeout(() => {
    if (getEntryStep() !== 'connecting') return;
    showConnectionError('连接超时，请检查网络或稍后重试', { showActions: true });
  }, 8000);
}

export function boot(): void {
  normalizeAuthHost();
  localStorage.setItem('uppy-game-url', window.location.href);
  initNickname();

  initEntryFlow();
  bindEntryUI(() => {
    submitSetupNickname();
  });

  initWaitingTips();
  bindReconnectRetry(() => {
    void connectWebSocket();
  });

  initConnection();
  requestAnimationFrame(gameLoop);

  window.addEventListener('game-ws-open', () => {
    onWebSocketOpen();
    updateUI({ force: true });
  });

  resizeCanvas();
  renderOnce();
}
