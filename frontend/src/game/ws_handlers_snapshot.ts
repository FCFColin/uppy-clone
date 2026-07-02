import { dispatch, getState } from './store.js';
import { updateInterpolation, freezeInterpolation, isDuplicateSeq } from './state_interp.js';
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

    const snapshotUpdate = applySnapshot(decoded);
    dispatch({ type: 'SET_STATE', partial: snapshotUpdate });

    if (shouldApplySnapshotPhase(decoded.phase)) {
      applyPhaseChange(decoded.phase);
    }

    if (decoded.ripples.length > 0) {
      const current = getState().ripples;
      dispatch({ type: 'SET_STATE', partial: {
        ripples: [...current.filter(r => r.isOptimistic), ...decoded.ripples],
      }});
    }

    if (decoded.wind !== undefined) {
      dispatch({ type: 'SET_STATE', partial: { wind: decoded.wind } });
      updateWindIndicator(decoded.wind);
    }

    updateScoresOnly();
    if (getState().pendingNickname) {
      const matched = getState().players.some((p) => p.nickname === getState().pendingNickname);
      if (matched) dispatch({ type: 'SET_STATE', partial: { pendingNickname: null } });
    }
    if (getState().phase === 'ended') {
      freezeInterpolation();
    } else {
      updateInterpolation(decoded.timestamp);
    }
    dispatch({ type: 'SET_STATE', partial: { hasReceivedFirstSnapshot: true } });
  } catch (e: unknown) {
    const errMsg: string = e instanceof Error ? e.message : String(e);
    console.error('[snapshot] parse error:', errMsg);
  }
}
