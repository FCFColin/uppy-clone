import { CLIENT_MSG } from '../shared/game/constants.js';
import { calculateCooldown } from './message_codec.js';
import { dispatch, getState } from './state.js';
import { sendOrQueue, getWs } from './ws_connection.js';
import { $canvas, clientToNormalized } from './renderer.js';
import { playTapSound } from '../shared/ui/audio.js';
import { updateUI } from './ui_update.js';

const PLAYER_INDEX_REJECTED = -1;
const PLAYER_INDEX_OPTIMISTIC = -2;

function createTapMessage(x: number, y: number): ArrayBuffer {
  const buf: ArrayBuffer = new ArrayBuffer(9);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.TAP);
  dv.setFloat32(1, x, true);
  dv.setFloat32(5, y, true);
  return buf;
}

export function handleTap(clientX: number, clientY: number): void {
  if (getState().phase !== 'playing') return;

  const { x, y } = clientToNormalized(clientX, clientY);

  const now: number = Date.now();
  if (now < getState().myCooldownEnd) {
    dispatch({ type: 'ADD_RIPPLE', ripple: { playerIndex: PLAYER_INDEX_REJECTED, x, y, time: now, rejected: true } });
    return;
  }

  dispatch({ type: 'SET_STATE', partial: { lastTapX: x, lastTapY: y } });

  const optimisticCooldown: number = calculateCooldown(getState().players.length || 1);
  dispatch({ type: 'SET_STATE', partial: { myCooldownEnd: now + optimisticCooldown } });

  dispatch({
    type: 'ADD_RIPPLE',
    ripple: { playerIndex: PLAYER_INDEX_OPTIMISTIC, x, y, time: now, isOptimistic: true },
  });

  sendOrQueue(createTapMessage(x, y));
  playTapSound();
}

export function tapAtBalloonCenter(): void {
  if (getState().phase !== 'playing') return;
  const rect = $canvas.getBoundingClientRect();
  if (!rect) return;
  handleTap(rect.left + rect.width * getState().balloon.x, rect.top + rect.height * (1 - getState().balloon.y));
}

export function requestRestart(): void {
  const $restartProgress: HTMLElement | null = document.getElementById('restart-progress');
  if (getState().phase !== 'ended') {
    if ($restartProgress) $restartProgress.textContent = '游戏尚未结束';
    return;
  }
  const ws = getWs();
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    dispatch({ type: 'SET_STATE', partial: { restartClicked: true } });
    if ($restartProgress) $restartProgress.textContent = '连接已断开，正在重连…';
    return;
  }
  dispatch({ type: 'SET_STATE', partial: { restartClicked: true } });
  if ($restartProgress) {
    $restartProgress.textContent = '正在提交重启投票...';
  }
  const buf: ArrayBuffer = new ArrayBuffer(1);
  new DataView(buf).setUint8(0, CLIENT_MSG.RESTART_VOTE);
  updateUI({ force: true });
  sendOrQueue(buf);
}
