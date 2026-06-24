import {
  MSG_TYPE, CLIENT_MSG,
  MAX_RECONNECT_ATTEMPTS, BASE_RECONNECT_DELAY,
  HEARTBEAT_INTERVAL_MS, HEARTBEAT_TIMEOUT_MS,
  MAX_PENDING_QUEUE,
  textDecoder,
} from './constants.js';
import { codeToPhase, encodeSetNickname } from './protocol.js';
import {
  state, updateInterpolation, resetInterpolation,
  isDuplicateSeq, seenSeqs, pendingQueue, resetClientState,
} from './state.js';
import {
  updateUI, startCountdownTimer,
  hideCountdownOverlay, showCountdownOverlay,
  hideLoadingOverlay, $lobbyCode, $hudCode,
} from './ui.js';
import { storeRefreshToken, getRefreshToken, refreshAccessToken } from '../shared/auth.js';

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
        console.warn('Heartbeat timeout, closing connection to reconnect');
        if (ws) ws.close();
      }, HEARTBEAT_TIMEOUT_MS);
    }
  }, HEARTBEAT_INTERVAL_MS);
}

function stopHeartbeat(): void {
  if (heartbeatInterval) {
    clearInterval(heartbeatInterval);
    heartbeatInterval = null;
  }
  if (heartbeatTimeout) {
    clearTimeout(heartbeatTimeout);
    heartbeatTimeout = null;
  }
}

function handlePong(): void {
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

function showReconnectBanner(attempt: number): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  const $text: HTMLElement | null = document.getElementById('reconnect-text');
  if ($text) $text.textContent = `网络断开，正在重连…（第${attempt}次尝试）`;
  if ($banner) $banner.classList.remove('hidden');
}

function hideReconnectBanner(): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  if ($banner) $banner.classList.add('hidden');
}

export function sendOrQueue(buffer: ArrayBuffer): void {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(buffer);
    console.log(`[sendOrQueue] sent directly (ws OPEN)`);
    return;
  }
  console.log(`[sendOrQueue] QUEUED (ws readyState=${ws ? ws.readyState : 'null'}, queue len=${pendingQueue.length + 1})`);
  pendingQueue.push(buffer);
  if (pendingQueue.length > MAX_PENDING_QUEUE) {
    pendingQueue.shift();
  }
}

function flushPendingQueue(): void {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  while (pendingQueue.length > 0) {
    const msg: ArrayBuffer | undefined = pendingQueue.shift();
    if (msg) ws.send(msg);
  }
}

async function ensureAuth(): Promise<boolean> {
  try {
    // 先尝试用 refresh token 刷新
    const refreshToken = getRefreshToken();
    if (refreshToken) {
      const refreshed = await refreshAccessToken();
      if (refreshed) {
        return true;
      }
    }

    // 刷新失败，走 quickplay 重新认证
    const savedNick: string = localStorage.getItem('uppy-nickname') || '';
    const res: Response = await fetch('/api/v1/auth/quickplay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ nickname: savedNick || undefined }),
    });
    if (res.ok) {
      const data: { userId?: string; refreshToken?: string } = await res.json() as { userId?: string; refreshToken?: string };
      if (data.userId) localStorage.setItem('uppy-player-id', data.userId);
      if (data.refreshToken) storeRefreshToken(data.refreshToken);
      return true;
    } else {
      console.error('Quickplay auth failed:', res.status);
      return false;
    }
  } catch (e: unknown) {
    console.error('Quickplay auth error:', e);
    return false;
  }
}

export function showConnectionError(message: string): void {
  const overlay: HTMLElement | null = document.getElementById('loading-overlay');
  if (!overlay) return;
  overlay.textContent = '';
  const container: HTMLDivElement = document.createElement('div');
  container.style.cssText = 'text-align:center;color:#fff;padding:2rem;';
  const icon: HTMLDivElement = document.createElement('div');
  icon.style.cssText = 'font-size:48px;margin-bottom:16px;';
  icon.textContent = '\u26A0';
  container.appendChild(icon);
  const msg: HTMLParagraphElement = document.createElement('p');
  msg.style.cssText = 'font-size:18px;margin-bottom:24px;';
  msg.textContent = message;
  container.appendChild(msg);
  const btn: HTMLButtonElement = document.createElement('button');
  btn.style.cssText = 'padding:0.8rem 2rem;font-size:1rem;cursor:pointer;border:none;border-radius:8px;background:#0f3460;color:#fff;';
  btn.textContent = '返回主页';
  btn.addEventListener('click', () => { location.href = '/'; });
  container.appendChild(btn);
  overlay.appendChild(container);
  overlay.style.animation = 'none';
  hideReconnectBanner();
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

export async function connectWebSocket(): Promise<void> {
  let savedPlayerId: string | null = localStorage.getItem('uppy-player-id');
  if (!savedPlayerId) {
    const authOk: boolean = await ensureAuth();
    if (!authOk) {
      showConnectionError('认证失败，请检查网络后重试');
      return;
    }
    savedPlayerId = localStorage.getItem('uppy-player-id');
  }

  const params: URLSearchParams = new URLSearchParams(window.location.search);
  let lobbyCode: string | null = params.get('code');

  if (!lobbyCode) {
    try {
      const res: Response = await fetch('/api/v1/registry/match', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });
      if (res.status === 401) {
        // Access token 过期，尝试刷新后重试
        const refreshed = await refreshAccessToken();
        if (refreshed) {
          const retryRes: Response = await fetch('/api/v1/registry/match', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
          });
          if (retryRes.ok) {
            const data: { lobbyCode: string } = await retryRes.json() as { lobbyCode: string };
            lobbyCode = data.lobbyCode;
          }
        }
      } else if (res.ok) {
        const data: { lobbyCode: string } = await res.json() as { lobbyCode: string };
        lobbyCode = data.lobbyCode;
      }
    } catch (e: unknown) {
      console.error('Failed to match room:', e);
    }
  }

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

  ws = new WebSocket(wsUrl);
  ws.binaryType = 'arraybuffer';
  window.__ws = ws;

  ws.onopen = () => {
    console.log('Connected to lobby:', lobbyCode);
    wsEverOpened = true;
    reconnectAttempts = 0;
    hideReconnectBanner();
    hideLoadingOverlay();
    startHeartbeat();
    flushPendingQueue();
    seenSeqs.clear();
    resetInterpolation();
    pendingQueue.length = 0;
    state.connectionError = null;
    reconnectTimer = null;
    if (state.phase === 'ended' && state.restartClicked) {
      console.log(`[ws-open] re-sending RESTART_VOTE after reconnect`);
      const restartBuf: ArrayBuffer = new ArrayBuffer(1);
      new DataView(restartBuf).setUint8(0, CLIENT_MSG.RESTART_VOTE);
      sendOrQueue(restartBuf);
    }
    const savedNick: string | null = localStorage.getItem('uppy-nickname');
    if (savedNick) {
      ws!.send(encodeSetNickname(savedNick));
    }
  };

  ws.onmessage = (event: MessageEvent) => {
    if (event.data instanceof ArrayBuffer) {
      handleBinaryMessage(event.data);
    }
  };

  ws.onclose = (event: CloseEvent) => {
    console.log(`[ws-close] disconnected, code=${event.code}, reason=${event.reason}, phase=${state.phase}`);
    stopHeartbeat();
    if (!wsEverOpened) {
      showConnectionError('连接失败，请重新进入');
      return;
    }
    scheduleReconnect();
  };

  ws.onerror = () => {
    console.error('WebSocket error');
  };
}

function scheduleReconnect(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    console.error('Max reconnect attempts reached. Please refresh the page.');
    showConnectionError('连接失败，请检查网络后重试');
    return;
  }
  const delay: number = Math.min(BASE_RECONNECT_DELAY * Math.pow(2, reconnectAttempts), 30000);
  reconnectAttempts++;
  console.log(`Reconnecting in ${delay}ms (attempt ${reconnectAttempts}/${MAX_RECONNECT_ATTEMPTS})...`);
  showReconnectBanner(reconnectAttempts);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connectWebSocket();
  }, delay);
}

export { stopHeartbeat };

export function getWs(): WebSocket | null {
  return ws;
}

function handleBinaryMessage(buffer: ArrayBuffer): void {
  const view: DataView = new DataView(buffer);
  const msgType: number = view.getUint8(0);
  console.log(`[ws-recv] msgType=${msgType} byteLength=${view.byteLength}`);

  switch (msgType) {
    case MSG_TYPE.SNAPSHOT:
      handleSnapshot(view);
      break;
    case MSG_TYPE.PLAYER_JOIN:
      handlePlayerJoin(view);
      break;
    case MSG_TYPE.PLAYER_LEAVE:
      handlePlayerLeave(view);
      break;
    case MSG_TYPE.TAP_ACCEPTED:
      handleTapAccepted(view);
      break;
    case MSG_TYPE.TAP_REJECTED:
      handleTapRejected();
      break;
    case MSG_TYPE.GAME_STATE_CHANGE:
      handleGameStateChange(view);
      break;
    case MSG_TYPE.RESTART_STATUS:
      handleRestartStatus(view);
      break;
    case MSG_TYPE.PONG:
      handlePong();
      break;
    default:
      console.warn('Unknown message type:', msgType);
  }
}

function handleSnapshot(view: DataView): void {
  try {
    if (view.byteLength < 37) {
      console.warn('[snapshot] message too short, ignoring');
      return;
    }

    let o: number = 1;
    const timestamp: number = view.getUint32(o, true); o += 4;

    if (isDuplicateSeq(timestamp)) {
      return;
    }

    state.score = view.getUint32(o, true); o += 4;
    const phaseCode: number = view.getUint8(o); o += 1;
    state.phase = codeToPhase(phaseCode);
    window.__gamePhase = state.phase;

    state.balloon.x = view.getFloat32(o, true); o += 4;
    state.balloon.y = view.getFloat32(o, true); o += 4;
    state.balloon.vy = view.getFloat32(o, true); o += 4;
    state.balloon.vx = view.getFloat32(o, true); o += 4;

    const birdActive: boolean = view.getUint8(o) === 1; o += 1;
    state.bird.active = birdActive;
    if (birdActive) {
      state.bird.x = view.getFloat32(o, true); o += 4;
      state.bird.y = view.getFloat32(o, true); o += 4;
    }

    state.ghost.active = view.getUint8(o) === 1; o += 1;
    state.ghost.x = view.getFloat32(o, true); o += 4;
    state.ghost.y = view.getFloat32(o, true); o += 4;
    state.ghost.repelTimer = view.getUint16(o, true); o += 2;
    console.log(`[snapshot] tick=${timestamp} phase=${state.phase} balloon=(${state.balloon.x.toFixed(4)},${state.balloon.y.toFixed(4)}) ghost.active=${state.ghost.active} ghost=(${state.ghost.x.toFixed(4)},${state.ghost.y.toFixed(4)})`);

    const playerCount: number = view.getUint8(o); o += 1;
    state.players = [];
    const now: number = Date.now();
    for (let i = 0; i < playerCount; i++) {
      const playerIndex: number = view.getUint16(o, true); o += 2;
      const cooldownRemainingMs: number = view.getUint32(o, true); o += 4;
      const palette: number = view.getUint32(o, true); o += 4;
      const scoreContribution: number = view.getUint32(o, true); o += 4;
      const nickLen: number = view.getUint8(o); o += 1;
      const nickname: string = textDecoder.decode(new Uint8Array(view.buffer, view.byteOffset + o, nickLen));
      o += nickLen;
      state.players.push({ playerIndex, cooldownEndTime: now + cooldownRemainingMs, palette, scoreContribution, nickname });
    }

    if (o < view.byteLength) {
      const rippleCount: number = view.getUint8(o); o += 1;
      for (let i = 0; i < rippleCount; i++) {
        const pIdx: number = view.getUint16(o, true); o += 2;
        const rx: number = view.getFloat32(o, true); o += 4;
        const ry: number = view.getFloat32(o, true); o += 4;
        state.ripples.push({ playerIndex: pIdx, x: rx, y: ry, time: Date.now() });
      }
    }

    if (o < view.byteLength) {
      state.wind = view.getFloat32(o, true); o += 4;
    }

    updateUI();
    updateInterpolation();
    state.hasReceivedFirstSnapshot = true;
  } catch (e: unknown) {
    const errMsg: string = e instanceof Error ? e.message : String(e);
    console.error('[snapshot] parse error:', errMsg);
  }
}

function handlePlayerJoin(view: DataView): void {
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const nicknameLen: number = view.getUint8(o); o += 1;
  const nickname: string = textDecoder.decode(new Uint8Array(view.buffer, view.byteOffset + o, nicknameLen));
  o += nicknameLen;
  const palette: number = view.getUint32(o, true);
  console.log('Player joined:', nickname, 'index:', playerIndex, 'palette:', palette);
}

function handlePlayerLeave(view: DataView): void {
  const playerIndex: number = view.getUint16(1, true);
  console.log('Player left, index:', playerIndex);
}

function handleTapAccepted(view: DataView): void {
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const cooldownRemainingMs: number = view.getUint32(o, true); o += 4;
  const x: number = view.getFloat32(o, true); o += 4;
  const y: number = view.getFloat32(o, true); o += 4;
  state.myCooldownEnd = Date.now() + cooldownRemainingMs;
  state.ripples = state.ripples.filter(r => !r.isOptimistic);
  state.ripples.push({ playerIndex, x, y, time: Date.now() });
  state.explosionEffect = { x, y, startTime: Date.now() };
}

function handleTapRejected(): void {
  if (state.lastTapX !== null) {
    state.ripples.push({
      playerIndex: -1,
      x: state.lastTapX,
      y: state.lastTapY!,
      time: Date.now(),
      rejected: true,
    });
  }
}

function handleGameStateChange(view: DataView): void {
  const phaseCode: number = view.getUint8(1);
  const prevPhase: string = state.phase;
  state.phase = codeToPhase(phaseCode);
  window.__gamePhase = state.phase;

  resetInterpolation();
  console.log(`[game-state-change] newPhase=${state.phase} prevPhase=${prevPhase}`);
  if (state.phase === 'playing') {
    console.log(`[game-state-change] phase=playing (restart or new game), clearing restart UI`);
    resetClientState();
    hideCountdownOverlay();
    if (state.countdownTimerInterval !== null) {
      clearInterval(state.countdownTimerInterval);
      state.countdownTimerInterval = null;
    }
    if (window._restartCountdownTimer) {
      clearInterval(window._restartCountdownTimer);
      window._restartCountdownTimer = null;
    }
    resetInterpolation();
  } else if (state.phase === 'countdown') {
    resetClientState();
    window.__gamePhase = 'countdown';
    resetInterpolation();
    showCountdownOverlay();
    startCountdownTimer(3);
    console.log('[game-state-change] phase=countdown, game starting in 3s');
  } else if (state.phase === 'ended') {
    try {
      const gActive: boolean = state.ghost && state.ghost.active;
      const gX: number = state.ghost ? state.ghost.x : 0;
      const gY: number = state.ghost ? state.ghost.y : 0;
      const bX: number = state.balloon ? state.balloon.x : 0;
      const bY: number = state.balloon ? state.balloon.y : 0;
      if (gActive) {
        const dx: number = bX - gX;
        const dy: number = bY - gY;
        const dist: number = Math.sqrt(dx * dx + dy * dy);
        if (dist < 0.15) {
          console.log('GHOST COLLISION DETECTED dist=' + dist.toFixed(4));
        } else {
          console.log('BALLOON GROUND HIT dist=' + dist.toFixed(4));
        }
      } else {
        console.log('BALLOON GROUND HIT no ghost active');
      }
    } catch (e: unknown) {
      const errMsg: string = e instanceof Error ? e.message : String(e);
      console.log('ENDED_DETECTION_ERROR ' + errMsg);
    }
    console.log('[game-state-change] phase=ended (GAME OVER received), showing ended screen');
    state.restartVotes = { yes: 0, total: state.players.length, countdownMs: 0 };
  }
  updateUI();
}

function handleRestartStatus(view: DataView): void {
  const yes: number = view.getUint8(1);
  const total: number = view.getUint8(2);
  const countdownMs: number = view.getUint32(3, true);
  state.restartVotes = {
    yes: yes,
    total: total,
    countdownMs: countdownMs,
    receivedAt: Date.now(),
  };
  if (countdownMs > 0 && !window._restartCountdownTimer) {
    window._restartCountdownTimer = setInterval(() => {
      if (state.restartVotes && state.restartVotes.countdownMs > 0) {
        const elapsed: number = Date.now() - (state.restartVotes.receivedAt ?? 0);
        const remaining: number = Math.max(0, state.restartVotes.countdownMs - elapsed);
        const $restartCountdown: HTMLElement | null = document.getElementById('restart-countdown');
        if ($restartCountdown && remaining > 0) {
          $restartCountdown.textContent = `${Math.ceil(remaining / 1000)} 秒后自动重启`;
        } else if ($restartCountdown) {
          $restartCountdown.textContent = '';
          clearInterval(window._restartCountdownTimer!);
          window._restartCountdownTimer = null;
        }
      }
    }, 1000);
  }
  updateUI();
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
