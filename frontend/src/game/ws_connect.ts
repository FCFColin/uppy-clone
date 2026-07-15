import { CLIENT_MSG } from '../shared/game/protocol.js';
import { dispatch, getState } from './store.js';
import { resetInterpolation } from './state_interp.js';
import { clearSeenSeqs } from './seen_seqs.js';
import { establishGameSession, sessionErrorMessage } from '../shared/network/session.js';
import {
  onLobbyCodeReady,
  onWebSocketOpen,
  onWebSocketClosed,
  getEntryStep,
} from './entry_flow.js';
import { clearWaitingInlineError } from './entry_flow_ui.js';
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
  startHeartbeat, stopHeartbeat, sendOrQueue, flushPendingQueue, clearOutboundQueue,
  hideReconnectBanner, scheduleReconnect, showConnectionError,
  wasRoomPreChecked, setRoomPreChecked,
} from './ws_connection.js';
import { enqueueBinaryMessage } from './ws_message_queue.js';
import { registerResetFn } from './reset_registry.js';

export { showConnectionError } from './ws_connection.js';

function shouldSkipConnect(lobbyCode: string): boolean {
  const ws = getWs();
  if (!ws) return false;
  if (getState().connectedLobbyCode !== lobbyCode) return false;
  return ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN;
}

async function resolveRoomCode(urlCode: string | null): Promise<string | null> {
  if (urlCode) {
    let freshMatch: string | null = null;
    try {
      freshMatch = sessionStorage.getItem('uppy-fresh-match');
    } catch {
      // sessionStorage may be unavailable
    }
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

function setupSocketHandlers(socket: WebSocket): void {
  socket.onopen = () => {
    const isReconnect = getWsEverOpened();
    setWsEverOpened(true);
    dispatch({ type: 'SET_STATE', partial: { wasEverConnected: true } });
    resetReconnectAttempts();
    hideReconnectBanner();
    clearWaitingInlineError();
    onWebSocketOpen();
    window.dispatchEvent(new Event('game-ws-open'));
    startHeartbeat();
    if (isReconnect) clearOutboundQueue();
    flushPendingQueue();
    clearSeenSeqs();
    resetInterpolation();
    dispatch({ type: 'SET_STATE', partial: { connectionError: null } });
    setReconnectTimer(null);
    if (getState().phase === 'ended' && getState().restartClicked) {
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
  dispatch({ type: 'SET_STATE', partial: { connectedLobbyCode: wsCode } });
  setWs(socket);
  setupSocketHandlers(socket);
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

  if (getState().wsConnectInFlight) return;
  dispatch({ type: 'SET_STATE', partial: { wsConnectInFlight: true } });

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
    dispatch({ type: 'SET_STATE', partial: { wsConnectInFlight: false } });
  }
}

/** Reset ws_connect module-level state for a new game session. */
export function resetWsConnectState(): void {
  dispatch({ type: 'SET_STATE', partial: { wsConnectInFlight: false, connectedLobbyCode: null } });
}

registerResetFn(resetWsConnectState);
