import { t } from '../i18n/t.js';
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
import { resetInterpolation, clearSeenSeqs } from './state_interp.js';
import { registerResetFn } from './reset_registry.js';
import { establishGameSession, sessionErrorMessage } from '../shared/network/network.js';
import { onLobbyCodeReady, onWebSocketOpen, onWebSocketClosed, getEntryStep } from './entry_flow.js';
import { clearWaitingInlineError } from './entry_flow.js';
import {
  resolveLobbyCode,
  getLobbyCodeFromUrl,
  validateRoomCode,
  roomErrorMessage,
  ROOM_CODE_RE,
} from './lobby_match.js';
import { safeGetItem } from '../shared/ui/ui.js';
import { handleBinaryMessage } from './ws_handlers.js';

const outboundMessageQueue: ArrayBuffer[] = [];

export function clearOutboundQueue(): void {
  outboundMessageQueue.length = 0;
}

export function getOutboundQueueLength(): number {
  return outboundMessageQueue.length;
}

function shiftOutbound(): ArrayBuffer | undefined {
  return outboundMessageQueue.shift();
}

function pushToOutbound(msg: ArrayBuffer, maxQueue: number): void {
  outboundMessageQueue.push(msg);
  if (outboundMessageQueue.length > maxQueue) {
    outboundMessageQueue.shift();
  }
}

function hasOutboundMessages(): boolean {
  return outboundMessageQueue.length > 0;
}

function requeueOutboundFront(msg: ArrayBuffer): void {
  outboundMessageQueue.unshift(msg);
}

const pendingBinaryMessages: ArrayBuffer[] = [];
const MAX_PENDING_BINARY = 32;

export function enqueueBinaryMessage(data: ArrayBuffer): void {
  pendingBinaryMessages.push(data);
  if (pendingBinaryMessages.length > MAX_PENDING_BINARY) {
    const dropped = pendingBinaryMessages.shift();
    if (dropped) {
      console.warn(`[ws] message queue full (${MAX_PENDING_BINARY}), dropping oldest message`);
    }
  }
}

export function drainPendingMessages(budget: number): void {
  let processed = 0;
  while (pendingBinaryMessages.length > 0 && processed < budget) {
    const data = pendingBinaryMessages.shift();
    if (data) {
      try {
        handleBinaryMessage(data);
      } catch (err: unknown) {
        console.error('[ws] message:', err);
      }
    }
    processed++;
  }
}

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
    showConnectionError(s.wasEverConnected ? t('error.conn_interrupted') : t('error.conn_failed'), {
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
          title: t('error.room_not_exist'),
        });
        return null;
      }
      setRoomPreChecked(true);
    }
    return urlCode;
  }

  const matched = await resolveLobbyCode();
  if (!matched) {
    showConnectionError(t('error.cannot_connect_server'), { showActions: true });
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
        if (code === 1006) message = t('error.conn_interrupted');
        else if (code === 1008) message = t('error.auth_failed');
        else if (code === 1011) message = t('error.server_error');
        else message = t('error.cannot_connect_room');
      } else {
        message = t('error.conn_failed');
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
    showConnectionError(t('error.invalid_room_code'), { showActions: true });
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

export function resetWsConnectState(): void {
  dispatch({ type: 'SET_STATE', partial: { wsConnectInFlight: false, connectedLobbyCode: null } });
}

registerResetFn(resetWsConnectState);
