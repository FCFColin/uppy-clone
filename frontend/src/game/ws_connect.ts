import { CLIENT_MSG } from './constants.js';
import { state, resetInterpolation, seenSeqs, outboundMessageQueue } from './state.js';
import { establishGameSession, sessionErrorMessage } from '../shared/session.js';
import { hideLoadingOverlay, $lobbyCode, $hudCode } from './ui.js';
import { resolveLobbyCode } from './ws_connect_lobby.js';
import {
  getLobbyCodeFromUrl,
  validateRoomCode,
  roomErrorMessage,
} from './room_validate.js';
import {
  setWs, getWsEverOpened, setWsEverOpened,
  resetReconnectAttempts, setReconnectTimer,
  startWsHeartbeat, stopHeartbeat, sendOrQueue, flushPendingQueue,
  hideReconnectBanner, scheduleReconnect, showConnectionError,
  wasRoomPreChecked, setRoomPreChecked,
} from './ws_connection.js';
import { enqueueBinaryMessage } from './ws_message_queue.js';

export { showConnectionError } from './ws_connection.js';

export async function connectWebSocket(): Promise<void> {
  const session = await establishGameSession();
  if (!session.ok) {
    showConnectionError(sessionErrorMessage(session));
    return;
  }
  const savedPlayerId: string | null = localStorage.getItem('uppy-player-id');

  const urlCode = getLobbyCodeFromUrl();
  if (urlCode) {
    const check = await validateRoomCode(urlCode);
    if (!check.ok) {
      showConnectionError(roomErrorMessage(check.reason), {
        showActions: true,
        title: check.reason === 'ended' ? '房间已结束' : '无法进入房间',
      });
      return;
    }
    setRoomPreChecked(true);
  }

  const lobbyCode: string | null = await resolveLobbyCode();
  if (!lobbyCode) {
    showConnectionError('无法连接到游戏服务器，请稍后重试', { showActions: true });
    return;
  }

  state.lobbyCode = lobbyCode;
  $lobbyCode.textContent = lobbyCode;
  $hudCode.textContent = lobbyCode;

  // 房间码已就绪即可进入昵称设置，WebSocket 在后台继续连接
  hideLoadingOverlay();
  window.dispatchEvent(new CustomEvent('game-lobby-ready'));

  const protocol: string = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const playerIdParam: string = savedPlayerId ? `?playerId=${savedPlayerId}` : '';
  const wsUrl: string = `${protocol}//${window.location.host}/api/v1/lobby/${lobbyCode}/ws${playerIdParam}`;

  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  setWs(socket);
  window.__ws = socket;

  socket.onopen = () => {
    setWsEverOpened(true);
    state.wasEverConnected = true;
    resetReconnectAttempts();
    hideReconnectBanner();
    hideLoadingOverlay();
    window.dispatchEvent(new Event('game-ws-open'));
    startWsHeartbeat();
    flushPendingQueue();
    seenSeqs.clear();
    resetInterpolation();
    outboundMessageQueue.length = 0;
    state.connectionError = null;
    setReconnectTimer(null);
    if (state.phase === 'ended' && state.restartClicked) {
      const restartBuf: ArrayBuffer = new ArrayBuffer(1);
      new DataView(restartBuf).setUint8(0, CLIENT_MSG.RESTART_VOTE);
      sendOrQueue(restartBuf);
    }
  };

	socket.onmessage = (event: MessageEvent) => {
		if (!(event.data instanceof ArrayBuffer)) return;
		enqueueBinaryMessage(event.data);
	};

  socket.onclose = () => {
    stopHeartbeat();
    if (!getWsEverOpened()) {
      const message = wasRoomPreChecked()
        ? '无法连接房间，请稍后重试'
        : '连接失败，请检查网络后重试';
      showConnectionError(message, { showActions: true });
      return;
    }
    scheduleReconnect();
  };

  socket.onerror = () => {
    console.error('WebSocket error');
  };
}
