import { PHYSICS } from '../shared/game/constants.js';
import type { InterpBirdPoint, InterpGhostPoint, InterpPoint, EntityAnchor } from './interp_buffers.js';
import {
  TELEPORT_THRESHOLD,
  TICK_MS,
  birdBuffer,
  clearAnchorBuffers,
  getRenderTime,
  ghostBuffer,
  pushAnchors,
  tryBalloonFromDelayBuffer,
  tryEntityFromDelayBuffer,
} from './interp_buffers.js';
import { dispatch, getState } from './state.js';
import { clearOutboundQueue, resetReconnectAttempts } from './ws_connection.js';
import { clearSeenSeqs } from './seen_seqs.js';
import { resetTutorial } from './tutorial.js';
import { resetWindHint, stopCooldownUpdater } from './ui_common.js';
import { resetUIUpdateCache } from './ui_update.js';
import { resetEntryFlowState } from './entry_flow.js';
import { runRegisteredResets } from './reset_registry.js';
import { clearRestartCountdownTimer } from './restart_vote_ui.js';

type ActivePoint = InterpGhostPoint;

let currSnapshotAt = 0;
let prevBalloon: InterpPoint | null = null;
let currBalloon: InterpPoint = { x: 0.5, y: 0.95 };
let currBalloonVx = 0;
let currBalloonVy = 0;
let prevGhost: InterpGhostPoint | null = null;
let currGhost: InterpGhostPoint = { x: 0.5, y: 0.5, active: false };
let prevBird: InterpBirdPoint | null = null;
let currBird: InterpBirdPoint = { x: 0, y: 0, active: false };
let lastRenderedBalloon: InterpPoint | null = null;

function lerpT(now: number): number {
  if (prevBalloon === null) return 1;
  return Math.max(0, (now - currSnapshotAt) / TICK_MS);
}

function interpolateBalloonPrevCurr(now: number): InterpPoint {
  if (prevBalloon === null) return currBalloon;
  const t = lerpT(now);
  const tClamp = Math.min(1, t);
  const pos = {
    x: prevBalloon.x + (currBalloon.x - prevBalloon.x) * tClamp,
    y: prevBalloon.y + (currBalloon.y - prevBalloon.y) * tClamp,
  };
  if (t > 1) {
    const extraTicks = t - 1;
    pos.x += currBalloonVx * extraTicks;
    pos.y += currBalloonVy * extraTicks - 0.5 * PHYSICS.GRAVITY * extraTicks * extraTicks;
  }
  return pos;
}

function interpolatePointPrevCurr(prev: InterpPoint, curr: InterpPoint, now: number): InterpPoint {
  const t = lerpT(now);
  const tClamp = Math.min(1, t);
  return {
    x: prev.x + (curr.x - prev.x) * tClamp,
    y: prev.y + (curr.y - prev.y) * tClamp,
  };
}

function isTeleport(v: { x: number; y: number }, curr: { x: number; y: number }): boolean {
  return Math.abs(v.x - curr.x) > TELEPORT_THRESHOLD || Math.abs(v.y - curr.y) > TELEPORT_THRESHOLD;
}

function resolvePrevBalloon(newBalloon: InterpPoint): InterpPoint {
  if (prevBalloon === null) return newBalloon;
  if (isTeleport(newBalloon, currBalloon)) {
    return { ...newBalloon };
  }
  if (lastRenderedBalloon !== null) {
    return { ...lastRenderedBalloon };
  }
  return { ...currBalloon };
}

export function updateInterpolation(tickCount: number): void {
  const s = getState();
  const newBalloon: InterpPoint = { x: s.balloon.x, y: s.balloon.y };
  const newGhost: InterpGhostPoint = { x: s.ghost.x, y: s.ghost.y, active: s.ghost.active };
  const newBird: InterpBirdPoint = { x: s.bird.x, y: s.bird.y, active: s.bird.active };

  if (prevBalloon === null) {
    prevBalloon = { ...newBalloon };
    currBalloon = { ...newBalloon };
    prevGhost = { ...newGhost };
    currGhost = { ...newGhost };
    prevBird = { ...newBird };
    currBird = { ...newBird };
    currBalloonVx = s.balloon.vx;
    currBalloonVy = s.balloon.vy;
    currSnapshotAt = Date.now();
    pushAnchors(tickCount);
    return;
  }

  prevBalloon = resolvePrevBalloon(newBalloon);
  prevGhost = isTeleport(newGhost, currGhost) ? { ...newGhost } : { ...currGhost };
  prevBird = isTeleport(newBird, currBird) ? { ...newBird } : { ...currBird };

  currBalloon = newBalloon;
  currGhost = newGhost;
  currBird = newBird;
  currBalloonVx = s.balloon.vx;
  currBalloonVy = s.balloon.vy;
  currSnapshotAt = Date.now();
  pushAnchors(tickCount);
}

function getInterpolatedEntityPrevCurr(
  curr: ActivePoint,
  prev: ActivePoint | null,
  now: number,
): ActivePoint | null {
  if (!curr.active) return null;
  if (prev === null) return curr;
  const pos = interpolatePointPrevCurr(prev, curr, now);
  return { x: pos.x, y: pos.y, active: true };
}

function getInterpolatedEntity(
  curr: ActivePoint,
  prev: ActivePoint | null,
  buffer: EntityAnchor[],
  now: number,
): ActivePoint | null {
  if (!curr.active) return null;
  const renderTime = getRenderTime();
  const buffered = tryEntityFromDelayBuffer(buffer, renderTime);
  if (buffered) return { x: buffered.x, y: buffered.y, active: true };
  return getInterpolatedEntityPrevCurr(curr, prev, now);
}

export function resetInterpolation(): void {
  prevBalloon = null;
  prevGhost = null;
  prevBird = null;
  currBalloon = { x: 0.5, y: 0.95 };
  currGhost = { x: 0.5, y: 0.5, active: false };
  currBird = { x: 0, y: 0, active: false };
  currBalloonVx = 0;
  currBalloonVy = 0;
  currSnapshotAt = 0;
  lastRenderedBalloon = null;
  clearAnchorBuffers();
}

export function freezeInterpolation(): void {
  const s = getState();
  const bx = s.balloon.x;
  const by = s.balloon.y;
  prevBalloon = { x: bx, y: by };
  currBalloon = { x: bx, y: by };
  prevGhost = { x: s.ghost.x, y: s.ghost.y, active: s.ghost.active };
  currGhost = { x: s.ghost.x, y: s.ghost.y, active: s.ghost.active };
  prevBird = { x: s.bird.x, y: s.bird.y, active: s.bird.active };
  currBird = { x: s.bird.x, y: s.bird.y, active: s.bird.active };
  lastRenderedBalloon = { x: bx, y: by };
}

export function getInterpolatedBalloon(now: number = Date.now()): InterpPoint {
  const buffered = tryBalloonFromDelayBuffer();
  if (buffered) return buffered;
  return interpolateBalloonPrevCurr(now);
}

export function getInterpolatedGhost(now: number = Date.now()): InterpGhostPoint | null {
  return getInterpolatedEntity(currGhost, prevGhost, ghostBuffer, now) as InterpGhostPoint | null;
}

export function getInterpolatedBird(now: number = Date.now()): InterpBirdPoint | null {
  return getInterpolatedEntity(currBird, prevBird, birdBuffer, now) as InterpBirdPoint | null;
}

export function commitRenderedState(now: number = Date.now()): void {
  lastRenderedBalloon = { ...getInterpolatedBalloon(now) };
}

export function getInterpState() {
  return { prevBalloon, currBalloon, prevGhost, currGhost };
}

export function resetRoundClientState(): void {
  dispatch({ type: 'RESET_ROUND' });
}

export function resetClientState(): void {
  dispatch({ type: 'RESET_ALL' });
  clearSeenSeqs();
  clearOutboundQueue();
  resetInterpolation();
  // ADR-025: reset all module-level mutable state to prevent cross-session leaks.
  // Modules with circular deps on this module (renderer, visual_helpers)
  // register their resets via reset_registry and are invoked below.
  resetTutorial();
  resetWindHint();
  resetReconnectAttempts();
  resetUIUpdateCache();
  resetEntryFlowState();
  stopCooldownUpdater();
  clearRestartCountdownTimer();
  runRegisteredResets();
}
