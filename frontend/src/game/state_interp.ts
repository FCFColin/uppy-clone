import { MAX_SEEN_SEQS } from './local_constants.js';
import { PHYSICS } from '../shared/game/constants.js';
import type { InterpBirdPoint, InterpGhostPoint, InterpPoint } from './interp_buffers.js';
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
import { state } from './state_types.js';

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

function resolvePrevBalloon(newBalloon: InterpPoint): InterpPoint {
  if (prevBalloon === null) return newBalloon;
  const dx = Math.abs(newBalloon.x - currBalloon.x);
  const dy = Math.abs(newBalloon.y - currBalloon.y);
  if (dx > TELEPORT_THRESHOLD || dy > TELEPORT_THRESHOLD) {
    return { ...newBalloon };
  }
  if (lastRenderedBalloon !== null) {
    return { ...lastRenderedBalloon };
  }
  return { ...currBalloon };
}

export function updateInterpolation(tickCount: number): void {
  const newBalloon: InterpPoint = { x: state.balloon.x, y: state.balloon.y };
  const newGhost: InterpGhostPoint = { x: state.ghost.x, y: state.ghost.y, active: state.ghost.active };
  const newBird: InterpBirdPoint = { x: state.bird.x, y: state.bird.y, active: state.bird.active };

  if (prevBalloon === null) {
    prevBalloon = { ...newBalloon };
    currBalloon = { ...newBalloon };
    prevGhost = { ...newGhost };
    currGhost = { ...newGhost };
    prevBird = { ...newBird };
    currBird = { ...newBird };
    currBalloonVx = state.balloon.vx;
    currBalloonVy = state.balloon.vy;
    currSnapshotAt = Date.now();
    pushAnchors(tickCount);
    return;
  }

  prevBalloon = resolvePrevBalloon(newBalloon);
  if (Math.abs(newGhost.x - currGhost.x) > TELEPORT_THRESHOLD || Math.abs(newGhost.y - currGhost.y) > TELEPORT_THRESHOLD) {
    prevGhost = { ...newGhost };
  } else {
    prevGhost = { ...currGhost };
  }
  if (Math.abs(newBird.x - currBird.x) > TELEPORT_THRESHOLD || Math.abs(newBird.y - currBird.y) > TELEPORT_THRESHOLD) {
    prevBird = { ...newBird };
  } else {
    prevBird = { ...currBird };
  }

  currBalloon = newBalloon;
  currGhost = newGhost;
  currBird = newBird;
  currBalloonVx = state.balloon.vx;
  currBalloonVy = state.balloon.vy;
  currSnapshotAt = Date.now();
  pushAnchors(tickCount);
}

function getInterpolatedGhostPrevCurr(now: number): InterpGhostPoint | null {
  if (!currGhost.active) return null;
  if (prevGhost === null) return currGhost;
  const pos = interpolatePointPrevCurr(prevGhost, currGhost, now);
  return { x: pos.x, y: pos.y, active: true };
}

function getInterpolatedBirdPrevCurr(now: number): InterpBirdPoint | null {
  if (!currBird.active) return null;
  if (prevBird === null) return currBird;
  const pos = interpolatePointPrevCurr(prevBird, currBird, now);
  return { x: pos.x, y: pos.y, active: true };
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
  const bx = state.balloon.x;
  const by = state.balloon.y;
  prevBalloon = { x: bx, y: by };
  currBalloon = { x: bx, y: by };
  prevGhost = { x: state.ghost.x, y: state.ghost.y, active: state.ghost.active };
  currGhost = { x: state.ghost.x, y: state.ghost.y, active: state.ghost.active };
  prevBird = { x: state.bird.x, y: state.bird.y, active: state.bird.active };
  currBird = { x: state.bird.x, y: state.bird.y, active: state.bird.active };
  lastRenderedBalloon = { x: bx, y: by };
}

export function getInterpolatedBalloon(now: number = Date.now()): InterpPoint {
  const buffered = tryBalloonFromDelayBuffer();
  if (buffered) return buffered;
  return interpolateBalloonPrevCurr(now);
}

export function getInterpolatedGhost(now: number = Date.now()): InterpGhostPoint | null {
  if (!currGhost.active) return null;
  const renderTime = getRenderTime();
  const buffered = tryEntityFromDelayBuffer(ghostBuffer, renderTime);
  if (buffered) return { x: buffered.x, y: buffered.y, active: true };
  return getInterpolatedGhostPrevCurr(now);
}

export function getInterpolatedBird(now: number = Date.now()): InterpBirdPoint | null {
  if (!currBird.active) return null;
  const renderTime = getRenderTime();
  const buffered = tryEntityFromDelayBuffer(birdBuffer, renderTime);
  if (buffered) return { x: buffered.x, y: buffered.y, active: true };
  return getInterpolatedBirdPrevCurr(now);
}

export function commitRenderedState(now: number = Date.now()): void {
  lastRenderedBalloon = { ...getInterpolatedBalloon(now) };
}

export const seenSeqs: Set<number> = new Set();

export function isDuplicateSeq(seq: number): boolean {
  if (seenSeqs.has(seq)) return true;
  seenSeqs.add(seq);
  if (seenSeqs.size > MAX_SEEN_SEQS) {
    const toRemove = Math.floor(MAX_SEEN_SEQS / 2);
    let i = 0;
    for (const s of seenSeqs) {
      seenSeqs.delete(s);
      i++;
      if (i >= toRemove) break;
    }
  }
  return false;
}

export const outboundMessageQueue: ArrayBuffer[] = [];

export function getInterpState(): {
  get prevBalloon(): InterpPoint | null;
  get currBalloon(): InterpPoint;
  get prevGhost(): InterpGhostPoint | null;
  get currGhost(): InterpGhostPoint;
} {
  return {
    get prevBalloon() { return prevBalloon; },
    get currBalloon() { return currBalloon; },
    get prevGhost() { return prevGhost; },
    get currGhost() { return currGhost; },
  };
}
