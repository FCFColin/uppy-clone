import {
  state, updateInterpolation, freezeInterpolation,
  isDuplicateSeq,
} from './state.js';
import { applyPhaseChange, shouldApplySnapshotPhase } from './phase_sync.js';
import { updateScoresOnly } from './ui_update.js';
import { updateWindIndicator } from './ui_wind.js';
import { applySnapshot, decodeSnapshot } from './message_codec.js';

export { shouldApplySnapshotPhase } from './phase_sync.js';

export function handleSnapshot(view: DataView): void {
  try {
    if (view.byteLength < 44) {
      console.warn('[snapshot] message too short, ignoring');
      return;
    }

    const decoded = decodeSnapshot(view);
    if (!decoded) {
      return;
    }

    if (isDuplicateSeq(decoded.timestamp)) {
      return;
    }

    applySnapshot(decoded, state);

    if (shouldApplySnapshotPhase(decoded.phase)) {
      applyPhaseChange(decoded.phase);
    }

    if (decoded.ripples.length > 0) {
      state.ripples = state.ripples.filter(r => r.isOptimistic);
      state.ripples.push(...decoded.ripples);
    }

    if (decoded.wind !== undefined) {
      state.wind = decoded.wind;
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
      updateInterpolation(decoded.timestamp);
    }
    state.hasReceivedFirstSnapshot = true;
  } catch (e: unknown) {
    const errMsg: string = e instanceof Error ? e.message : String(e);
    console.error('[snapshot] parse error:', errMsg);
  }
}
