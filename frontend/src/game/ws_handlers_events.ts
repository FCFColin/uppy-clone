import { state } from './state.js';
import { pushFloatingText } from './visual_helpers.js';

export function handleTapAccepted(view: DataView): void {
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const cooldownRemainingMs: number = view.getUint32(o, true); o += 4;
  const _balloonX: number = view.getFloat32(o, true); o += 4;
  const _balloonY: number = view.getFloat32(o, true);
  state.myCooldownEnd = Date.now() + cooldownRemainingMs;
  state.ripples = state.ripples.filter(r => !r.isOptimistic);
  const tapX = state.lastTapX ?? _balloonX;
  const tapY = state.lastTapY ?? _balloonY;
  state.ripples.push({ playerIndex, x: tapX, y: tapY, time: Date.now() });
  state.explosionEffect = { x: tapX, y: tapY, startTime: Date.now() };
}

export function handleTapRejected(): void {
  state.myCooldownEnd = 0;
  state.ripples = state.ripples.filter(r => !r.isOptimistic);
  if (state.lastTapX !== null) {
    state.ripples.push({
      playerIndex: -1,
      x: state.lastTapX,
      y: state.lastTapY!,
      time: Date.now(),
      rejected: true,
    });
    pushFloatingText(state.lastTapX, state.lastTapY!, '太远了');
  }
}
