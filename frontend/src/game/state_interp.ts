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
import { getState } from './store.js';

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

const seenSeqs: Set<number> = new Set();

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

export function clearSeenSeqs(): void {
  seenSeqs.clear();
}

/** Exposed for testing. Use isDuplicateSeq and clearSeenSeqs for all production access. */
export function getSeenSeqsSize(): number {
  return seenSeqs.size;
}

export function getInterpState() {
  return { prevBalloon, currBalloon, prevGhost, currGhost };
}
