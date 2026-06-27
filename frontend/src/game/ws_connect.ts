import { CLIENT_MSG } from './constants.js';
import { state, resetInterpolation, seenSeqs, pendingQueue } from './state.js';
import { establishGameSession, sessionErrorMessage } from '../shared/session.js';
import { hideLoadingOverlay, $lobbyCode, $hudCode } from './ui.js';
import { resolveLobbyCode } from './ws_connect_lobby.js';
import {
  setWs, getWsEverOpened, setWsEverOpened,
  resetReconnectAttempts, setReconnectTimer,
  startWsHeartbeat, stopHeartbeat, sendOrQueue, flushPendingQueue,
  hideReconnectBanner, scheduleReconnect, showConnectionError,
} from './ws_connection.js';
import { handleBinaryMessage } from './ws_handlers.js';

export { showConnectionError } from './ws_connection.js';

export async function connectWebSocket(): Promise<void> {
  const session = await establishGameSession();
  if (!session.ok) {
    showConnectionError(sessionErrorMessage(session));
    return;
  }
  const savedPlayerId: string | null = localStorage.getItem('uppy-player-id');

  const lobbyCode: string | null = await resolveLobbyCode();
  if (!lobbyCode) {
    showConnectionError('无法连接到游戏服务器，请稍后重试');
    return;
  }

  state.lobbyCode = lobbyCode;
  $lobbyCode.textContent = lobbyCode;
  $hudCode.textContent = lobbyCode;

  const protocol: string = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const playerIdParam: string = savedPlayerId ? `?playerId=${savedPlayerId}` : '';
  const wsUrl: string = `${protocol}//${window.location.host}/api/v1/lobby/${lobbyCode}/ws${playerIdParam}`;

  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  setWs(socket);
  window.__ws = socket;

  socket.onopen = () => {
    setWsEverOpened(true);
    resetReconnectAttempts();
    hideReconnectBanner();
    hideLoadingOverlay();
    startWsHeartbeat();
    flushPendingQueue();
    seenSeqs.clear();
    resetInterpolation();
    pendingQueue.length = 0;
    state.connectionError = null;
    setReconnectTimer(null);
    if (state.phase === 'ended' && state.restartClicked) {
      const restartBuf: ArrayBuffer = new ArrayBuffer(1);
      new DataView(restartBuf).setUint8(0, CLIENT_MSG.RESTART_VOTE);
      sendOrQueue(restartBuf);
    }
  };

  socket.onmessage = (event: MessageEvent) => {
    if (event.data instanceof ArrayBuffer) {
      handleBinaryMessage(event.data);
    }
  };

  socket.onclose = () => {
    stopHeartbeat();
    if (!getWsEverOpened()) {
      showConnectionError('连接失败，请重新进入');
      return;
    }
    scheduleReconnect();
  };

  socket.onerror = () => {
    console.error('WebSocket error');
  };
}
