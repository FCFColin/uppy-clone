import { dispatch } from './store.js';
import { seenSeqs, outboundMessageQueue, resetInterpolation } from './state_interp.js';

/** Clear per-round gameplay FX (phase transitions). */
export function resetRoundClientState(): void {
  dispatch({ type: 'RESET_ROUND' });
}

/** Full client reset including snapshot readiness and outbound queue. */
export function resetClientState(): void {
  dispatch({ type: 'RESET_ALL' });
  seenSeqs.clear();
  outboundMessageQueue.length = 0;
  resetInterpolation();
}
