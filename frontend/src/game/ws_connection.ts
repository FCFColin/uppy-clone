import {
  CLIENT_MSG,
  MAX_RECONNECT_ATTEMPTS, BASE_RECONNECT_DELAY,
  HEARTBEAT_INTERVAL_MS, HEARTBEAT_TIMEOUT_MS,
  MAX_PENDING_QUEUE,
} from './constants.js';
import { pendingQueue } from './state.js';
import { matchNewRoomCode } from './room_validate.js';

let ws: WebSocket | null = null;
let reconnectAttempts: number = 0;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let wsEverOpened: boolean = false;
let roomPreChecked: boolean = false;
let errorActionsBound = false;
let heartbeatInterval: ReturnType<typeof setInterval> | null = null;
let heartbeatTimeout: ReturnType<typeof setTimeout> | null = null;
let lastPingTime: number = 0;

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

export function setRoomPreChecked(value: boolean): void {
  roomPreChecked = value;
}

export function wasRoomPreChecked(): boolean {
  return roomPreChecked;
}

function bindErrorPanelActions(): void {
  if (errorActionsBound) return;
  errorActionsBound = true;

  const backBtn = document.getElementById('loading-back-btn');
  if (backBtn) {
    backBtn.addEventListener('click', () => {
      window.location.href = '/';
    });
  }

  const matchBtn = document.getElementById('loading-match-btn');
  if (matchBtn) {
    matchBtn.addEventListener('click', () => {
      void (async () => {
        matchBtn.setAttribute('disabled', 'true');
        const code = await matchNewRoomCode();
        if (code) {
          window.location.href = `/play.html?code=${code}`;
          return;
        }
        matchBtn.removeAttribute('disabled');
        const errorText = document.getElementById('loading-error-text');
        if (errorText) {
          errorText.textContent = '匹配失败，请稍后重试或返回大厅';
        }
      })();
    });
  }
}

export interface ConnectionErrorOptions {
  showActions?: boolean;
  title?: string;
}

function errorTitleForMessage(message: string): string {
  if (message.includes('已结束')) return '房间已结束';
  if (message.includes('不存在')) return '无法进入房间';
  if (message.includes('超时') || message.includes('网络') || message.includes('连接')) return '连接失败';
  return '无法进入房间';
}

export function showConnectionError(message: string, options?: ConnectionErrorOptions): void {
  const overlay = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.dataset.error = 'true';
  overlay.style.display = 'flex';

  const spinner = overlay.querySelector('.loading-spinner') as HTMLElement | null;
  const loadingText = overlay.querySelector('.loading-text') as HTMLElement | null;
  const errorPanel = document.getElementById('loading-error-panel');
  const errorTitle = document.getElementById('loading-error-title');
  const errorText = document.getElementById('loading-error-text');
  const actions = document.getElementById('loading-error-actions');

  if (spinner) spinner.classList.add('hidden');
  if (loadingText) loadingText.classList.add('hidden');
  if (errorTitle) errorTitle.textContent = options?.title ?? errorTitleForMessage(message);
  if (errorText) errorText.textContent = message;
  if (errorPanel) errorPanel.classList.remove('hidden');
  if (actions) {
    if (options?.showActions) {
      actions.classList.remove('hidden');
    } else {
      actions.classList.add('hidden');
    }
  }

  bindErrorPanelActions();
  hideReconnectBanner();
  clearReconnectTimer();
}

export function setReconnectTimer(timer: ReturnType<typeof setTimeout> | null): void {
  reconnectTimer = timer;
}

export function scheduleReconnect(): void {
  clearReconnectTimer();
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    showConnectionError('连接失败，请检查网络后重试', { showActions: true });
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
