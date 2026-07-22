import { CLIENT_MSG } from '../shared/game/constants.js';
import {
  MAX_RECONNECT_ATTEMPTS,
  BASE_RECONNECT_DELAY,
  HEARTBEAT_INTERVAL_MS,
  HEARTBEAT_TIMEOUT_MS,
  MAX_PENDING_QUEUE,
} from './constants.js';
import {
  showReconnectBanner,
  showConnectionError as showConnectionErrorUI,
  hideReconnectBanner,
  type ConnectionErrorOptions,
} from './ui_common.js';
import { getState, dispatch } from './state.js';
import { resetInterpolation } from './state_interp.js';
import { clearSeenSeqs } from './seen_seqs.js';
import { establishGameSession, sessionErrorMessage } from '../shared/network/session.js';
import { onLobbyCodeReady, onWebSocketOpen, onWebSocketClosed, getEntryStep } from './entry_flow.js';
import { clearWaitingInlineError } from './entry_flow.js';
import {
  resolveLobbyCode,
  getLobbyCodeFromUrl,
  validateRoomCode,
  roomErrorMessage,
  ROOM_CODE_RE,
} from './lobby_match.js';
import { registerResetFn } from './reset_registry.js';
import { safeGetItem } from '../shared/ui/utils.js';
import {
  clearOutboundQueue,
  enqueueBinaryMessage,
  pushToOutbound,
  shiftOutbound,
  hasOutboundMessages,
  requeueOutboundFront,
} from './ws_message_queue.js';

// Re-export queue functions for downstream consumers importing from './ws_connection.js'
export {
  clearOutboundQueue,
  getOutboundQueueLength,
  enqueueBinaryMessage,
  drainPendingMessages,
} from './ws_message_queue.js';

// --- Connection State ---
/**
 * 单一连接状态对象（v2-R-47）
 *
 * 收敛原先 8 个模块级 `let` 变量（ws/reconnectAttempts/reconnectTimer/
 * wsEverOpened/roomPreChecked/heartbeatInterval/heartbeatTimeout/lastPingTime），
 * 状态集中管理便于追踪与测试。外部 setter/getter API 保持不变。
 */
interface ConnectionState {
  ws: WebSocket | null;
  reconnectAttempts: number;
  reconnectTimer: ReturnType<typeof setTimeout> | null;
  wsEverOpened: boolean;
  roomPreChecked: boolean;
  heartbeatInterval: ReturnType<typeof setInterval> | null;
  heartbeatTimeout: ReturnType<typeof setTimeout> | null;
  lastPingTime: number;
}

const connectionState: ConnectionState = {
  ws: null,
  reconnectAttempts: 0,
  reconnectTimer: null,
  wsEverOpened: false,
  roomPreChecked: false,
  heartbeatInterval: null,
  heartbeatTimeout: null,
  lastPingTime: 0,
};

export function showConnectionError(message: string, options?: ConnectionErrorOptions): void {
  clearReconnectTimer();
  showConnectionErrorUI(message, options);
}

export function startHeartbeat(): void {
  stopHeartbeat();
  connectionState.heartbeatInterval = setInterval(() => {
    if (connectionState.ws && connectionState.ws.readyState === WebSocket.OPEN) {
      connectionState.lastPingTime = Date.now();
      connectionState.ws.send(new Uint8Array([CLIENT_MSG.PING]).buffer);
      if (connectionState.heartbeatTimeout) {
        clearTimeout(connectionState.heartbeatTimeout);
        connectionState.heartbeatTimeout = null;
      }
      const socketAtPing: WebSocket | null = connectionState.ws;
      connectionState.heartbeatTimeout = setTimeout(() => {
        if (connectionState.ws === socketAtPing) connectionState.ws.close();
      }, HEARTBEAT_TIMEOUT_MS);
    }
  }, HEARTBEAT_INTERVAL_MS);
}

export function stopHeartbeat(): void {
  if (connectionState.heartbeatInterval) {
    clearInterval(connectionState.heartbeatInterval);
    connectionState.heartbeatInterval = null;
  }
  if (connectionState.heartbeatTimeout) {
    clearTimeout(connectionState.heartbeatTimeout);
    connectionState.heartbeatTimeout = null;
  }
}

export function handlePong(): void {
  if (connectionState.heartbeatTimeout) {
    clearTimeout(connectionState.heartbeatTimeout);
    connectionState.heartbeatTimeout = null;
  }
}

export function sendOrQueue(buffer: ArrayBuffer): void {
  if (connectionState.ws && connectionState.ws.readyState === WebSocket.OPEN) {
    connectionState.ws.send(buffer);
    return;
  }
  pushToOutbound(buffer, MAX_PENDING_QUEUE);
}

export function flushPendingQueue(): void {
  if (!connectionState.ws || connectionState.ws.readyState !== WebSocket.OPEN) return;
  while (hasOutboundMessages()) {
    const msg: ArrayBuffer | undefined = shiftOutbound();
    if (msg) {
      try {
        connectionState.ws.send(msg);
      } catch (e: unknown) {
        console.error('[ws] flush send error, re-queueing message:', e);
        requeueOutboundFront(msg);
        break;
      }
    }
  }
}

export function getWs(): WebSocket | null {
  return connectionState.ws;
}

export function setWs(socket: WebSocket | null): void {
  connectionState.ws = socket;
}

export function getWsEverOpened(): boolean {
  return connectionState.wsEverOpened;
}

export function setWsEverOpened(value: boolean): void {
  connectionState.wsEverOpened = value;
}

export function resetReconnectAttempts(): void {
  connectionState.reconnectAttempts = 0;
}

export function clearReconnectTimer(): void {
  if (connectionState.reconnectTimer !== null) {
    clearTimeout(connectionState.reconnectTimer);
    connectionState.reconnectTimer = null;
  }
}

export function setRoomPreChecked(value: boolean): void {
  connectionState.roomPreChecked = value;
}

export function wasRoomPreChecked(): boolean {
  return connectionState.roomPreChecked;
}

export function setReconnectTimer(timer: ReturnType<typeof setTimeout> | null): void {
  connectionState.reconnectTimer = timer;
}

export function scheduleReconnect(): void {
  clearReconnectTimer();
  if (connectionState.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    const s = getState();
    showConnectionError(s.wasEverConnected ? '对局连接已中断，请检查网络后重试' : '连接失败，请检查网络后重试', {
      showActions: true,
      midGameDisconnect: s.wasEverConnected,
    });
    return;
  }
  const delay = Math.min(BASE_RECONNECT_DELAY * Math.pow(2, connectionState.reconnectAttempts), 30000);
  connectionState.reconnectAttempts++;
  showReconnectBanner(connectionState.reconnectAttempts);
  connectionState.reconnectTimer = setTimeout(() => {
    connectionState.reconnectTimer = null;
    void connectWebSocket().catch((e: unknown) => {
      console.error('reconnect failed:', e);
    });
  }, delay);
}

export function waitForWebSocket(maxWaitMs = 5000): Promise<boolean> {
  return new Promise<boolean>((resolve: (ok: boolean) => void) => {
    if (connectionState.ws && connectionState.ws.readyState === WebSocket.OPEN) return resolve(true);
    const ws = connectionState.ws;
    if (!ws) {
      resolve(false);
      return;
    }
    const onOpen = (): void => {
      ws.removeEventListener('open', onOpen);
      resolve(true);
    };
    ws.addEventListener('open', onOpen);
    setTimeout(() => {
      ws.removeEventListener('open', onOpen);
      resolve(false);
    }, maxWaitMs);
  });
}
// --- WebSocket Connect Logic ---

function shouldSkipConnect(lobbyCode: string): boolean {
  const ws = getWs();
  if (!ws) return false;
  if (getState().connectedLobbyCode !== lobbyCode) return false;
  return ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN;
}

async function resolveRoomCode(urlCode: string | null): Promise<string | null> {
  if (urlCode) {
    const freshMatch: string | null = safeGetItem('uppy-fresh-match', sessionStorage);
    if (freshMatch === urlCode) {
      sessionStorage.removeItem('uppy-fresh-match');
      setRoomPreChecked(true);
    } else {
      const check = await validateRoomCode(urlCode);
      if (!check.ok) {
        showConnectionError(roomErrorMessage(check.reason), {
          showActions: true,
          title: '无法进入房间',
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

  socket.onclose = (event?: CloseEvent) => {
    stopHeartbeat();
    onWebSocketClosed();
    if (!getWsEverOpened()) {
      let message: string;
      if (wasRoomPreChecked()) {
        const code = event?.code;
        if (code === 1006) message = '连接中断，请检查网络后重试';
        else if (code === 1008) message = '认证失败，请刷新页面重试';
        else if (code === 1011) message = '服务器内部错误，请稍后重试';
        else message = '无法连接房间，请稍后重试';
      } else {
        message = '连接失败，请检查网络后重试';
      }
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

  if (lobbyCode && shouldSkipConnect(lobbyCode)) {
    return;
  }

  if (getState().wsConnectInFlight) return;
  dispatch({ type: 'SET_STATE', partial: { wsConnectInFlight: true } });

  try {
    const [session, resolvedCode] = await Promise.all([establishGameSession(), resolveRoomCode(urlCode)]);

    if (!session.ok) {
      showConnectionError(sessionErrorMessage(session), { showActions: true });
      return;
    }
    lobbyCode = resolvedCode;
    if (!lobbyCode) return;

    // 推迟 onLobbyCodeReady 到 establishGameSession/resolveRoomCode 都成功：
    // 失败时 entryStep 保持 'connecting'，showConnectionError 才能正确显示错误面板
    // 而不被 nickname-setup-screen 的 entry-overlay-active 盖住。
    if (urlCode && getEntryStep() === 'connecting') {
      onLobbyCodeReady(lobbyCode);
    }

    if (shouldSkipConnect(lobbyCode)) {
      return;
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
