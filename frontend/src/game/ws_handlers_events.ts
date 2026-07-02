import { dispatch, getState } from './store.js';
import { pushFloatingText } from './visual_helpers.js';

export function handleTapAccepted(view: DataView): void {
  let o: number = 1;
  const playerIndex: number = view.getUint16(o, true); o += 2;
  const cooldownRemainingMs: number = view.getUint32(o, true); o += 4;
  const _balloonX: number = view.getFloat32(o, true); o += 4;
  const _balloonY: number = view.getFloat32(o, true);
  dispatch({ type: 'SET_STATE', partial: { myCooldownEnd: Date.now() + cooldownRemainingMs } });
  const tapX = getState().lastTapX ?? _balloonX;
  const tapY = getState().lastTapY ?? _balloonY;
  dispatch({ type: 'SET_STATE', partial: {
    ripples: [...getState().ripples.filter(r => !r.isOptimistic), { playerIndex, x: tapX, y: tapY, time: Date.now() }],
    explosionEffect: { x: tapX, y: tapY, startTime: Date.now() },
  }});
}

export function handleTapRejected(): void {
  const lastTapX = getState().lastTapX;
  const lastTapY = getState().lastTapY;
  const remaining = getState().ripples.filter(r => !r.isOptimistic);
  if (lastTapX !== null) {
    remaining.push({
      playerIndex: -1,
      x: lastTapX,
      y: lastTapY!,
      time: Date.now(),
      rejected: true,
    });
    pushFloatingText(lastTapX, lastTapY!, '太远了');
  }
  dispatch({ type: 'SET_STATE', partial: { myCooldownEnd: 0, ripples: remaining } });
}
