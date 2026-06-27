import { textDecoder } from './constants.js';
import { codeToPhase } from './message_codec.js';
import {
  state, updateInterpolation, freezeInterpolation,
  isDuplicateSeq,
} from './state.js';
import { applyPhaseChange, shouldApplySnapshotPhase } from './phase_sync.js';
import { updateScoresOnly } from './ui_update.js';
import { updateWindIndicator } from './ui_wind.js';

export { shouldApplySnapshotPhase } from './phase_sync.js';

export function handleSnapshot(view: DataView): void {
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
    const snapshotPhase = codeToPhase(phaseCode);
    if (shouldApplySnapshotPhase(snapshotPhase)) {
      applyPhaseChange(snapshotPhase);
    }

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
      const snapshotRipples = [];
      for (let i = 0; i < rippleCount; i++) {
        const pIdx: number = view.getUint16(o, true); o += 2;
        const rx: number = view.getFloat32(o, true); o += 4;
        const ry: number = view.getFloat32(o, true); o += 4;
        snapshotRipples.push({ playerIndex: pIdx, x: rx, y: ry, time: Date.now() });
      }
      if (snapshotRipples.length > 0) {
        state.ripples = state.ripples.filter(r => r.isOptimistic);
        state.ripples.push(...snapshotRipples);
      }
    }

    if (o < view.byteLength) {
      state.wind = view.getFloat32(o, true); o += 4;
      updateWindIndicator(state.wind);
    }

    updateScoresOnly();
    if (state.pendingNickname) {
      const matched = state.players.some((p) => p.nickname === state.pendingNickname);
      if (matched) state.pendingNickname = null;
    }
    if (state.phase === 'ended') {
      freezeInterpolation();
    } else {
      updateInterpolation(timestamp);
    }
    state.hasReceivedFirstSnapshot = true;
  } catch (e: unknown) {
    const errMsg: string = e instanceof Error ? e.message : String(e);
    console.error('[snapshot] parse error:', errMsg);
  }
}
