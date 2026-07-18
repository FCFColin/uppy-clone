import { encodeSetNickname } from './message_codec.js';
import { dispatch } from './state.js';
import { normalizeAuthHost } from '../shared/network/session.js';
import { showToast } from '../shared/ui/utils.js';
import { resizeCanvas, startGameLoop, renderOnce } from './renderer.js';
import { updateUI } from './ui_update.js';
import { pickRandomNickname, $setupNicknameInput } from './ui_elements.js';
import { initWaitingTips, bindReconnectRetry } from './ui_common.js';
import { connectWebSocket, showConnectionError } from './ws_connection.js';
import { sendOrQueue } from './ws_connection.js';
import {
  initEntryFlow, bindEntryUI, onNicknameSubmit, onWebSocketOpen, getEntryStep,
} from './entry_flow.js';
import { safeGetItem, safeSetItem } from '../shared/ui/utils.js';

function submitSetupNickname(): void {
  const input: HTMLInputElement | null = document.getElementById('setup-nickname-input') as HTMLInputElement | null;
  let nickname: string = input ? input.value.trim() : '';
  if (!nickname) {
    nickname = pickRandomNickname();
  }
  safeSetItem('uppy-nickname', nickname);
  dispatch({ type: 'SET_STATE', partial: { pendingNickname: nickname } });
  onNicknameSubmit();

  const msg: ArrayBuffer = encodeSetNickname(nickname);
  sendOrQueue(msg);

  updateUI({ force: true });
  showToast(`欢迎，${nickname}！`);
}

function initNickname(): void {
  const savedNickname: string | null = safeGetItem('uppy-nickname');
  if (savedNickname && $setupNicknameInput) {
    $setupNicknameInput.value = savedNickname;
  } else if ($setupNicknameInput) {
    $setupNicknameInput.value = pickRandomNickname();
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
  safeSetItem('uppy-game-url', window.location.href);
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
