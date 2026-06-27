import {
  CLIENT_MSG,
  MAX_RECONNECT_ATTEMPTS, BASE_RECONNECT_DELAY,
  HEARTBEAT_INTERVAL_MS, HEARTBEAT_TIMEOUT_MS,
  MAX_PENDING_QUEUE,
} from './constants.js';
import { outboundMessageQueue } from './state.js';
import {
  hideReconnectBanner, showReconnectBanner, updatePingDisplay,
  showConnectionError as showConnectionErrorUI, type ConnectionErrorOptions,
} from './connection_ui.js';

export { type ConnectionErrorOptions } from './connection_ui.js';
export { hideReconnectBanner } from './connection_ui.js';

let ws: WebSocket | null = null;
let reconnectAttempts: number = 0;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let wsEverOpened: boolean = false;
let roomPreChecked: boolean = false;
let heartbeatInterval: ReturnType<typeof setInterval> | null = null;
let heartbeatTimeout: ReturnType<typeof setTimeout> | null = null;
let lastPingTime: number = 0;

export function showConnectionError(message: string, options?: ConnectionErrorOptions): void {
  clearReconnectTimer();
  showConnectionErrorUI(message, options);
}

function startHeartbeat(): void {
  stopHeartbeat();
  heartbeatInterval = setInterval(() => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      lastPingTime = Date.now();
      ws.send(new Uint8Array([CLIENT_MSG.PING]).buffer);
      if (heartbeatTimeout) {
        clearTimeout(heartbeatTimeout);
        heartbeatTimeout = null;
      }
      heartbeatTimeout = setTimeout(() => {
        if (ws) ws.close();
      }, HEARTBEAT_TIMEOUT_MS);
    }
  }, HEARTBEAT_INTERVAL_MS);
}

export function stopHeartbeat(): void {
  if (heartbeatInterval) {
    clearInterval(heartbeatInterval);
    heartbeatInterval = null;
  }
  if (heartbeatTimeout) {
    clearTimeout(heartbeatTimeout);
    heartbeatTimeout = null;
  }
}

export function startWsHeartbeat(): void {
  startHeartbeat();
}

export function handlePong(): void {
  if (heartbeatTimeout) {
    clearTimeout(heartbeatTimeout);
    heartbeatTimeout = null;
  }
  if (lastPingTime > 0) {
    updatePingDisplay(Date.now() - lastPingTime);
  }
}

export function sendOrQueue(buffer: ArrayBuffer): void {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(buffer);
    return;
  }
  outboundMessageQueue.push(buffer);
  if (outboundMessageQueue.length > MAX_PENDING_QUEUE) {
    outboundMessageQueue.shift();
  }
}

export function flushPendingQueue(): void {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  while (outboundMessageQueue.length > 0) {
    const msg: ArrayBuffer | undefined = outboundMessageQueue.shift();
    if (msg) ws.send(msg);
  }
}

export function getWs(): WebSocket | null {
  return ws;
}

export function setWs(socket: WebSocket | null): void {
  ws = socket;
}

export function getWsEverOpened(): boolean {
  return wsEverOpened;
}

export function setWsEverOpened(value: boolean): void {
  wsEverOpened = value;
}

export function resetReconnectAttempts(): void {
  reconnectAttempts = 0;
}

export function clearReconnectTimer(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

export function setRoomPreChecked(value: boolean): void {
  roomPreChecked = value;
}

export function wasRoomPreChecked(): boolean {
  return roomPreChecked;
}

export function setReconnectTimer(timer: ReturnType<typeof setTimeout> | null): void {
  reconnectTimer = timer;
}

export function scheduleReconnect(): void {
  clearReconnectTimer();
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    void import('./state.js').then(({ state: s }) => {
      showConnectionError(
        s.wasEverConnected ? '对局连接已中断，请检查网络后重试' : '连接失败，请检查网络后重试',
        { showActions: true, midGameDisconnect: s.wasEverConnected },
      );
    });
    return;
  }
  const delay: number = Math.min(BASE_RECONNECT_DELAY * Math.pow(2, reconnectAttempts), 30000);
  reconnectAttempts++;
  showReconnectBanner(reconnectAttempts);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    void import('./ws_connect.js').then((m) => m.connectWebSocket());
  }, delay);
}

export function waitForWebSocket(maxWaitMs: number = 5000): Promise<void> {
  return new Promise((resolve: () => void) => {
    if (ws && ws.readyState === WebSocket.OPEN) return resolve();
    const start: number = Date.now();
    const check: ReturnType<typeof setInterval> = setInterval(() => {
      if ((ws && ws.readyState === WebSocket.OPEN) || Date.now() - start > maxWaitMs) {
        clearInterval(check);
        resolve();
      }
    }, 100);
  });
}
