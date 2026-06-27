import {
  CLIENT_MSG,
  MAX_RECONNECT_ATTEMPTS, BASE_RECONNECT_DELAY,
  HEARTBEAT_INTERVAL_MS, HEARTBEAT_TIMEOUT_MS,
  MAX_PENDING_QUEUE,
} from './constants.js';
import { pendingQueue } from './state.js';

let ws: WebSocket | null = null;
let reconnectAttempts: number = 0;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let wsEverOpened: boolean = false;
let heartbeatInterval: ReturnType<typeof setInterval> | null = null;
let heartbeatTimeout: ReturnType<typeof setTimeout> | null = null;
let lastPingTime: number = 0;

function startHeartbeat(): void {
  stopHeartbeat();
  heartbeatInterval = setInterval(() => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      lastPingTime = Date.now();
      ws.send(new Uint8Array([CLIENT_MSG.PING]).buffer);
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
    const rtt: number = Date.now() - lastPingTime;
    const $ping: HTMLElement | null = document.getElementById('ping-display');
    if ($ping) $ping.textContent = `${rtt}ms`;
  }
}

export function hideReconnectBanner(): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  if ($banner) $banner.classList.add('hidden');
}

function showReconnectBanner(attempt: number): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  const $text: HTMLElement | null = document.getElementById('reconnect-text');
  if ($text) $text.textContent = `网络断开，正在重连…（第${attempt}次尝试）`;
  if ($banner) $banner.classList.remove('hidden');
}

export function sendOrQueue(buffer: ArrayBuffer): void {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(buffer);
    return;
  }
  pendingQueue.push(buffer);
  if (pendingQueue.length > MAX_PENDING_QUEUE) {
    pendingQueue.shift();
  }
}

export function flushPendingQueue(): void {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  while (pendingQueue.length > 0) {
    const msg: ArrayBuffer | undefined = pendingQueue.shift();
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

export function showConnectionError(message: string): void {
  const overlay = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.dataset.error = 'true';
  overlay.style.display = 'flex';
  overlay.textContent = '';
  const msg = document.createElement('p');
  msg.textContent = message;
  msg.style.cssText = 'font-size:18px;margin-bottom:24px;color:#fff;text-align:center;padding:2rem;';
  overlay.appendChild(msg);
  hideReconnectBanner();
  clearReconnectTimer();
}

export function setReconnectTimer(timer: ReturnType<typeof setTimeout> | null): void {
  reconnectTimer = timer;
}

export function scheduleReconnect(): void {
  clearReconnectTimer();
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    showConnectionError('连接失败，请检查网络后重试');
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
