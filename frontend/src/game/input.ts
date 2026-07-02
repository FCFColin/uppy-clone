import { CLIENT_MSG } from './constants.js';
import { calculateCooldown } from './message_codec.js';
import { state } from './state.js';
import { sendOrQueue, getWs } from './websocket.js';
import { clientToNormalized } from './renderer_canvas.js';
import { playTapSound } from '../shared/ui/audio.js';
import { updateUI } from './ui.js';

export function handleTap(clientX: number, clientY: number): void {
  if (state.phase !== 'playing') return;

  const { x, y } = clientToNormalized(clientX, clientY);

  const now: number = Date.now();
  if (now < state.myCooldownEnd) {
    state.ripples.push({ playerIndex: -1, x, y, time: now, rejected: true });
    return;
  }

  state.lastTapX = x;
  state.lastTapY = y;

  const optimisticCooldown: number = calculateCooldown(state.players.length || 1);
  state.myCooldownEnd = now + optimisticCooldown;

  state.ripples.push({ playerIndex: -2, x, y, time: now, isOptimistic: true });

  const buf: ArrayBuffer = new ArrayBuffer(9);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.TAP);
  dv.setFloat32(1, x, true);
  dv.setFloat32(5, y, true);
  sendOrQueue(buf);
  playTapSound();
}

export function tapAtBalloonCenter(): void {
  if (state.phase !== 'playing') return;
  const canvas = document.getElementById('game-canvas');
  const rect = canvas?.getBoundingClientRect();
  if (!rect) return;
  handleTap(rect.left + rect.width * state.balloon.x, rect.top + rect.height * (1 - state.balloon.y));
}

export function requestRestart(): void {
  const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
  if (state.phase !== 'ended') {
    if ($restartProgress) $restartProgress.textContent = '游戏尚未结束';
    return;
  }
  const ws = getWs();
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    state.restartClicked = true;
    if ($restartProgress) $restartProgress.textContent = '连接已断开，正在重连…';
    updateUI(true);
    return;
  }
  state.restartClicked = true;
  if ($restartProgress) {
    $restartProgress.textContent = '正在提交重启投票...';
  }
  updateUI(true);
  const buf: ArrayBuffer = new ArrayBuffer(1);
  new DataView(buf).setUint8(0, CLIENT_MSG.RESTART_VOTE);
  sendOrQueue(buf);
}
