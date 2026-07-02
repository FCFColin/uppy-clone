import { CLIENT_MSG } from './constants.js';
import { state, resetInterpolation, seenSeqs } from './state.js';
import { establishGameSession, sessionErrorMessage } from '../shared/network/session.js';
import {
  onLobbyCodeReady,
  onWebSocketOpen,
  onWebSocketClosed,
  clearWaitingInlineError,
  getEntryStep,
} from './entry_flow.js';
import { resolveLobbyCode } from './lobby_match.js';
import {
  getLobbyCodeFromUrl,
  validateRoomCode,
  roomErrorMessage,
  ROOM_CODE_RE,
} from './room_validate.js';
import {
  setWs, getWs, getWsEverOpened, setWsEverOpened,
  resetReconnectAttempts, setReconnectTimer,
  startHeartbeat, stopHeartbeat, sendOrQueue, flushPendingQueue,
  hideReconnectBanner, scheduleReconnect, showConnectionError,
  wasRoomPreChecked, setRoomPreChecked,
} from './ws_connection.js';
import { enqueueBinaryMessage } from './ws_message_queue.js';

export { showConnectionError } from './ws_connection.js';

let connectInFlight = false;
let connectedLobbyCode: string | null = null;

function shouldSkipConnect(lobbyCode: string): boolean {
  const ws = getWs();
  if (!ws) return false;
  if (connectedLobbyCode !== lobbyCode) return false;
  return ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN;
}

async function resolveRoomCode(urlCode: string | null): Promise<string | null> {
  if (urlCode) {
    const freshMatch = sessionStorage.getItem('uppy-fresh-match');
    if (freshMatch === urlCode) {
      sessionStorage.removeItem('uppy-fresh-match');
      setRoomPreChecked(true);
    } else {
      const check = await validateRoomCode(urlCode);
      if (!check.ok) {
        showConnectionError(roomErrorMessage(check.reason), {
          showActions: true,
          title: check.reason === 'ended' ? '房间已结束' : '无法进入房间',
        });
        return null;
      }
      setRoomPreChecked(true);
    }
    return urlCode;
  }

  const matched = await resolveLobbyCode();
  if (!matched) {
    showConnectionError('无法连接到游戏服务器，请稍后重试', { showActions: true });
    return null;
  }
  onLobbyCodeReady(matched);
  return matched;
}

function openGameSocket(wsCode: string): void {
  const existing = getWs();
  if (existing && existing.readyState !== WebSocket.CLOSED) {
    existing.onclose = null;
    existing.close();
  }

  if (!ROOM_CODE_RE.test(wsCode)) {
    showConnectionError('无效的房间码', { showActions: true });
    return;
  }

  const protocol: string = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl: string = `${protocol}//${window.location.host}/api/v1/lobby/${wsCode}/ws`;

  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  connectedLobbyCode = wsCode;
  setWs(socket);

  socket.onopen = () => {
    setWsEverOpened(true);
    state.wasEverConnected = true;
    resetReconnectAttempts();
    hideReconnectBanner();
    clearWaitingInlineError();
    onWebSocketOpen();
    window.dispatchEvent(new Event('game-ws-open'));
    startHeartbeat();
    flushPendingQueue();
    seenSeqs.clear();
    resetInterpolation();
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
    onWebSocketClosed();
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

export async function connectWebSocket(): Promise<void> {
  const urlCode = getLobbyCodeFromUrl();
  let lobbyCode: string | null = urlCode;

  if (lobbyCode && getEntryStep() === 'connecting') {
    onLobbyCodeReady(lobbyCode);
  }

  if (lobbyCode && shouldSkipConnect(lobbyCode)) {
    return;
  }

  if (connectInFlight) return;
  connectInFlight = true;

  try {
    const [session, resolvedCode] = await Promise.all([
      establishGameSession(),
      resolveRoomCode(urlCode),
    ]);

    if (!session.ok) {
      showConnectionError(sessionErrorMessage(session));
      return;
    }
    lobbyCode = resolvedCode;
    if (!lobbyCode) return;

    if (shouldSkipConnect(lobbyCode)) {
      return; /* v8 ignore next -- defensive skip after async resolve */
    }

    openGameSocket(lobbyCode);
  } finally {
    connectInFlight = false;
  }
}
