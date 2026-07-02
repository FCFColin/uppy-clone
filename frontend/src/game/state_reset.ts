import { state } from './state_types.js';
import { seenSeqs, outboundMessageQueue, resetInterpolation } from './state_interp.js';

/** Clear per-round gameplay FX (phase transitions). */
export function resetRoundClientState(): void {
  state.ripples = [];
  state.explosionEffect = null;
  state.myCooldownEnd = 0;
  state.lastTapX = null;
  state.lastTapY = null;
  state.restartClicked = false;
  state.restartVotes = { yes: 0, total: 0, countdownMs: 0 };
  state.score = 0;
  state.balloon = { x: 0.5, y: 0.95, vx: 0, vy: 0 };
  state.bird = { x: 0, y: 0, active: false };
  state.ghost = { x: 0, y: 0, active: false, repelTimer: 0 };
  state.wind = 0;
}

/** Full client reset including snapshot readiness and outbound queue. */
export function resetClientState(): void {
  state.hasReceivedFirstSnapshot = false;
  seenSeqs.clear();
  state.score = 0;
  state.myCooldownEnd = 0;
  state.ripples = [];
  state.lastTapX = null;
  state.lastTapY = null;
  state.balloon = { x: 0.5, y: 0.5, vx: 0, vy: 0 };
  state.bird = { x: 0, y: 0, active: false };
  state.ghost = { x: 0, y: 0, active: false, repelTimer: 0 };
  state.wind = 0;
  state.explosionEffect = null;
  outboundMessageQueue.length = 0;
  state.restartClicked = false;
  state.restartVotes = { yes: 0, total: 0, countdownMs: 0 };
  resetInterpolation();
}
