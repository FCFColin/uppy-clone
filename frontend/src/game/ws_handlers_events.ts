import { textDecoder } from './constants.js';
import { state } from './state.js';

export function handlePlayerJoin(view: DataView): void {
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const nicknameLen: number = view.getUint8(o); o += 1;
  const nickname: string = textDecoder.decode(new Uint8Array(view.buffer, view.byteOffset + o, nicknameLen));
  o += nicknameLen;
  const palette: number = view.getUint32(o, true);
  console.log('Player joined:', nickname, 'index:', playerIndex, 'palette:', palette);
}

export function handlePlayerLeave(view: DataView): void {
  const playerIndex: number = view.getUint16(1, true);
  console.log('Player left, index:', playerIndex);
}

export function handleTapAccepted(view: DataView): void {
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const cooldownRemainingMs: number = view.getUint32(o, true); o += 4;
  // server sends balloon position; we use local tap position for visual effects
  const _balloonX: number = view.getFloat32(o, true); o += 4;
  const _balloonY: number = view.getFloat32(o, true); o += 4;
  state.myCooldownEnd = Date.now() + cooldownRemainingMs;
  state.ripples = state.ripples.filter(r => !r.isOptimistic);
  const tapX = state.lastTapX ?? _balloonX;
  const tapY = state.lastTapY ?? _balloonY;
  state.ripples.push({ playerIndex, x: tapX, y: tapY, time: Date.now() });
  state.explosionEffect = { x: tapX, y: tapY, startTime: Date.now() };
}

export function handleTapRejected(): void {
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
