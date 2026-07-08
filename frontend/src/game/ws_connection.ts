import { CLIENT_MSG } from '../shared/game/protocol.js';
import {
  MAX_RECONNECT_ATTEMPTS, BASE_RECONNECT_DELAY,
  HEARTBEAT_INTERVAL_MS, HEARTBEAT_TIMEOUT_MS,
  MAX_PENDING_QUEUE,
} from './local_constants.js';
import {
  showReconnectBanner, updatePingDisplay,
  showConnectionError as showConnectionErrorUI, type ConnectionErrorOptions,
} from './connection_ui.js';

export { type ConnectionErrorOptions } from './connection_ui.js';
export { hideReconnectBanner } from './connection_ui.js';

const outboundMessageQueue: ArrayBuffer[] = [];
export function clearOutboundQueue(): void {
  outboundMessageQueue.length = 0;
}
export function getOutboundQueueLength(): number {
  return outboundMessageQueue.length;
}

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
      connectionState.heartbeatTimeout = setTimeout(() => {
        if (connectionState.ws) connectionState.ws.close();
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
  if (connectionState.lastPingTime > 0) {
    updatePingDisplay(Date.now() - connectionState.lastPingTime);
  }
}

export function sendOrQueue(buffer: ArrayBuffer): void {
  if (connectionState.ws && connectionState.ws.readyState === WebSocket.OPEN) {
    connectionState.ws.send(buffer);
    return;
  }
  outboundMessageQueue.push(buffer);
  if (outboundMessageQueue.length > MAX_PENDING_QUEUE) {
    outboundMessageQueue.shift();
  }
}

export function flushPendingQueue(): void {
  if (!connectionState.ws || connectionState.ws.readyState !== WebSocket.OPEN) return;
  while (outboundMessageQueue.length > 0) {
    const msg: ArrayBuffer | undefined = outboundMessageQueue.shift();
    if (msg) connectionState.ws.send(msg); /* v8 ignore else -- shift only returns undefined on empty queue */
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
    void import('./state_types.js').then(({ state: s }) => {
      showConnectionError(
        s.wasEverConnected ? '对局连接已中断，请检查网络后重试' : '连接失败，请检查网络后重试',
        { showActions: true, midGameDisconnect: s.wasEverConnected },
      );
    });
    return;
  }
  const delay = Math.min(BASE_RECONNECT_DELAY * Math.pow(2, connectionState.reconnectAttempts), 30000);
  connectionState.reconnectAttempts++;
  showReconnectBanner(connectionState.reconnectAttempts);
  connectionState.reconnectTimer = setTimeout(() => {
    connectionState.reconnectTimer = null;
    void import('./ws_connect.js').then((m) => m.connectWebSocket());
  }, delay);
}

export function waitForWebSocket(maxWaitMs = 5000): Promise<boolean> {
  let cancelled = false;
  return new Promise<boolean>((resolve: (ok: boolean) => void) => {
    if (connectionState.ws && connectionState.ws.readyState === WebSocket.OPEN) return resolve(true);
    const start = Date.now();
    const check: ReturnType<typeof setInterval> = setInterval(() => {
      if (cancelled) { clearInterval(check); return; }
      if (connectionState.ws && connectionState.ws.readyState === WebSocket.OPEN) {
        clearInterval(check);
        resolve(true);
      } else if (Date.now() - start > maxWaitMs) {
        clearInterval(check);
        resolve(false);
      }
    }, 100);
  });
}
