import { encodeSetNickname } from './message_codec.js';
import { dispatch } from './store.js';
import { normalizeAuthHost } from '../shared/network/session.js';
import { showToast } from '../shared/ui/toast.js';
import { resizeCanvas, startGameLoop, renderOnce } from './renderer.js';
import { updateUI } from './ui_update.js';
import { generateRandomNickname } from './ui_utils.js';
import { $setupNicknameInput } from './ui_elements.js';
import { connectWebSocket, showConnectionError } from './ws_connect.js';
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
  try {
    localStorage.setItem('uppy-nickname', nickname);
  } catch {
    // localStorage may be unavailable (private browsing, quota)
  }
  dispatch({ type: 'SET_STATE', partial: { pendingNickname: nickname } });
  onNicknameSubmit();

  const msg: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(msg);

  updateUI({ force: true });
  showToast(`欢迎，${nickname}！`);
}

function initNickname(): void {
  let savedNickname: string | null = null;
  try {
    savedNickname = localStorage.getItem('uppy-nickname');
  } catch {
    // localStorage may be unavailable
  }
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

let bootBound = false;

/** Reset boot guard (test use only). */
export function resetBootBound(): void {
  bootBound = false;
}

export function boot(): void {
  if (bootBound) return;
  bootBound = true;

  normalizeAuthHost();
  try {
    localStorage.setItem('uppy-game-url', window.location.href);
  } catch {
    // localStorage may be unavailable (private browsing, quota)
  }
  initNickname();

  initEntryFlow();
  bindEntryUI(submitSetupNickname);

  initWaitingTips();
  bindReconnectRetry(() => {
    void connectWebSocket();
  });

  initConnection();
  startGameLoop();

  window.addEventListener('game-ws-open', () => {
    onWebSocketOpen();
    updateUI({ force: true });
  });

  resizeCanvas();
  renderOnce();
}
