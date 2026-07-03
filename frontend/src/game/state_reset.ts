import { dispatch } from './store.js';
import { resetInterpolation, clearSeenSeqs } from './state_interp.js';
import { clearOutboundQueue } from './ws_connection.js';

/** Clear per-round gameplay FX (phase transitions). */
export function resetRoundClientState(): void {
  dispatch({ type: 'RESET_ROUND' });
}

/** Full client reset including snapshot readiness and outbound queue. */
export function resetClientState(): void {
  dispatch({ type: 'RESET_ALL' });
  clearSeenSeqs();
  clearOutboundQueue();
  resetInterpolation();
}
