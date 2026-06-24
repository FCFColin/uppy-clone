import { CLIENT_MSG } from './constants.js';
import { calculateCooldown } from './protocol.js';
import { state } from './state.js';
import { sendOrQueue } from './websocket.js';
import { $canvas } from './renderer.js';

export function handleTap(clientX: number, clientY: number): void {
  if (state.phase !== 'playing') return;

  const rect: DOMRect = $canvas.getBoundingClientRect();
  const x: number = (clientX - rect.left) / rect.width;
  const y: number = 1 - ((clientY - rect.top) / rect.height);

  const now: number = Date.now();
  if (now < state.myCooldownEnd) {
    state.ripples.push({ playerIndex: -1, x, y, time: now, rejected: true });
    return;
  }

  state.lastTapX = x;
  state.lastTapY = y;

  const optimisticCooldown: number = calculateCooldown(state.players.length || 1);
  state.myCooldownEnd = now + optimisticCooldown;

  state.ripples.push({ playerIndex: -2, x, y, time: now, optimistic: true, isOptimistic: true });

  const buf: ArrayBuffer = new ArrayBuffer(9);
  const dv: DataView = new DataView(buf);
  dv.setUint8(0, CLIENT_MSG.TAP);
  dv.setFloat32(1, x, true);
  dv.setFloat32(5, y, true);
  sendOrQueue(buf);
}

export function requestRestart(): void {
  console.log(`[restart] button clicked, current phase=${state.phase}`);
  if (state.phase !== 'ended') {
    console.log(`[restart] REJECTED: phase is ${state.phase}, not 'ended'`);
    return;
  }
  state.restartClicked = true;
  const buf: ArrayBuffer = new ArrayBuffer(1);
  new DataView(buf).setUint8(0, CLIENT_MSG.RESTART_VOTE);
  console.log(`[restart] sending RESTART_VOTE message`);
  sendOrQueue(buf);
  console.log(`[restart] RESTART_VOTE sent/queued`);
}
