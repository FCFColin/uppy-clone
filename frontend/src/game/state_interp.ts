import { MAX_SEEN_SEQS, PHYSICS } from './constants.js';
import { state } from './state_types.js';

const TICK_MS: number = PHYSICS.TICK_INTERVAL;
const INTERP_DELAY_MS: number = PHYSICS.INTERP_DELAY_MS;
const MAX_SNAPSHOT_BUFFER = 12;
const TELEPORT_THRESHOLD = 0.05;

interface InterpPoint {
  x: number;
  y: number;
}

interface InterpGhostPoint {
  x: number;
  y: number;
  active: boolean;
}

interface InterpBirdPoint {
  x: number;
  y: number;
  active: boolean;
}

interface BalloonAnchor {
  tick: number;
  receivedAt: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

interface EntityAnchor {
  tick: number;
  receivedAt: number;
  x: number;
  y: number;
  active: boolean;
}

let prevTick: number = 0;
let currTick: number = 0;
let currSnapshotAt: number = 0;
let prevBalloon: InterpPoint | null = null;
let currBalloon: InterpPoint = { x: 0.5, y: 0.95 };
let currBalloonVx = 0;
let currBalloonVy = 0;
let prevGhost: InterpGhostPoint | null = null;
let currGhost: InterpGhostPoint = { x: 0.5, y: 0.5, active: false };
let prevBird: InterpBirdPoint | null = null;
let currBird: InterpBirdPoint = { x: 0, y: 0, active: false };
let lastRenderedBalloon: InterpPoint | null = null;

const balloonBuffer: BalloonAnchor[] = [];
const ghostBuffer: EntityAnchor[] = [];
const birdBuffer: EntityAnchor[] = [];

/** Kept for API compatibility; RTT is shown in HUD only. */
export function setInterpolationClockOffset(_rttMs: number): void {}

function getRenderTime(): number {
  return Date.now() - INTERP_DELAY_MS;
}

function lerpT(): number {
  if (prevBalloon === null) return 1;
  return Math.max(0, (Date.now() - currSnapshotAt) / TICK_MS);
}

function pushAnchors(tickCount: number): void {
  const receivedAt = Date.now();
  balloonBuffer.push({
    tick: tickCount,
    receivedAt,
    x: state.balloon.x,
    y: state.balloon.y,
    vx: state.balloon.vx,
    vy: state.balloon.vy,
  });
  ghostBuffer.push({
    tick: tickCount,
    receivedAt,
    x: state.ghost.x,
    y: state.ghost.y,
    active: state.ghost.active,
  });
  birdBuffer.push({
    tick: tickCount,
    receivedAt,
    x: state.bird.x,
    y: state.bird.y,
    active: state.bird.active,
  });
  while (balloonBuffer.length > MAX_SNAPSHOT_BUFFER) balloonBuffer.shift();
  while (ghostBuffer.length > MAX_SNAPSHOT_BUFFER) ghostBuffer.shift();
  while (birdBuffer.length > MAX_SNAPSHOT_BUFFER) birdBuffer.shift();
}

function clearAnchorBuffers(): void {
  balloonBuffer.length = 0;
  ghostBuffer.length = 0;
  birdBuffer.length = 0;
}

function findAnchorIndex(buffer: { receivedAt: number }[], renderTime: number): number {
  let index = -1;
  for (let i = 0; i < buffer.length; i++) {
    if (buffer[i]!.receivedAt <= renderTime) index = i;
  }
  return index;
}

function tryBalloonFromDelayBuffer(): InterpPoint | null {
  if (balloonBuffer.length === 0) return null;
  const renderTime = getRenderTime();
  const i = findAnchorIndex(balloonBuffer, renderTime);
  if (i < 0) return null;

  const a = balloonBuffer[i]!;
  const b = balloonBuffer[i + 1];
  if (!b) {
    const ticks = (renderTime - a.receivedAt) / TICK_MS;
    return { x: a.x + a.vx * ticks, y: a.y + a.vy * ticks };
  }

  const span = b.receivedAt - a.receivedAt;
  if (span <= 0) return { x: b.x, y: b.y };
  const t = Math.min(1, (renderTime - a.receivedAt) / span);
  let x = a.x + (b.x - a.x) * t;
  let y = a.y + (b.y - a.y) * t;
  if (t >= 1) {
    const extraTicks = (renderTime - b.receivedAt) / TICK_MS;
    if (extraTicks > 0) {
      x += b.vx * extraTicks;
      y += b.vy * extraTicks;
    }
  }
  return { x, y };
}

// --- Entity delay-buffer interpolation (ghost / bird) ---

function tryEntityFromDelayBuffer(
  buffer: EntityAnchor[],
  renderTime: number,
): InterpPoint | null {
  if (buffer.length === 0) return null;
  const i = findAnchorIndex(buffer, renderTime);
  if (i < 0) return null;

  const a = buffer[i]!;
  const b = buffer[i + 1];
  if (!a.active) return null;
  if (!b) return { x: a.x, y: a.y };
  if (!b.active) return { x: a.x, y: a.y };

  const span = b.receivedAt - a.receivedAt;
  if (span <= 0) return { x: b.x, y: b.y };
  const t = Math.min(1, (renderTime - a.receivedAt) / span);
  return {
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
  };
}

function interpolateBalloonPrevCurr(): InterpPoint {
  if (prevBalloon === null) return currBalloon;
  const t = lerpT();
  const tClamp = Math.min(1, t);
  const pos: InterpPoint = {
    x: prevBalloon.x + (currBalloon.x - prevBalloon.x) * tClamp,
    y: prevBalloon.y + (currBalloon.y - prevBalloon.y) * tClamp,
  };
  if (t > 1) {
    const extraTicks = t - 1;
    pos.x += currBalloonVx * extraTicks;
    pos.y += currBalloonVy * extraTicks;
  }
  return pos;
}

function interpolatePointPrevCurr(prev: InterpPoint, curr: InterpPoint): InterpPoint {
  const t = lerpT();
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
    prevTick = tickCount;
    currTick = tickCount;
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

  prevTick = currTick;
  currTick = tickCount;
  currBalloon = newBalloon;
  currGhost = newGhost;
  currBird = newBird;
  currBalloonVx = state.balloon.vx;
  currBalloonVy = state.balloon.vy;
  currSnapshotAt = Date.now();
  pushAnchors(tickCount);
}

function getInterpolatedGhostPrevCurr(): InterpGhostPoint | null {
  if (!currGhost.active) return null;
  if (prevGhost === null) return currGhost;
  const pos = interpolatePointPrevCurr(prevGhost, currGhost);
  return { x: pos.x, y: pos.y, active: true };
}

function getInterpolatedBirdPrevCurr(): InterpBirdPoint | null {
  if (!currBird.active) return null;
  if (prevBird === null) return currBird;
  const pos = interpolatePointPrevCurr(prevBird, currBird);
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
  currTick = 0;
  prevTick = 0;
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
  prevTick = currTick;
  lastRenderedBalloon = { x: bx, y: by };
}

export function getInterpolatedBalloon(): InterpPoint {
  const buffered = tryBalloonFromDelayBuffer();
  if (buffered) return buffered;
  return interpolateBalloonPrevCurr();
}

export function getInterpolatedGhost(): InterpGhostPoint | null {
  if (!currGhost.active) return null;
  const renderTime = getRenderTime();
  const buffered = tryEntityFromDelayBuffer(ghostBuffer, renderTime);
  if (buffered) return { x: buffered.x, y: buffered.y, active: true };
  return getInterpolatedGhostPrevCurr();
}

export function getInterpolatedBird(): InterpBirdPoint | null {
  if (!currBird.active) return null;
  const renderTime = getRenderTime();
  const buffered = tryEntityFromDelayBuffer(birdBuffer, renderTime);
  if (buffered) return { x: buffered.x, y: buffered.y, active: true };
  return getInterpolatedBirdPrevCurr();
}

/** Call once per rendered frame to avoid snap-back on the next snapshot. */
export function commitRenderedState(): void {
  lastRenderedBalloon = { ...getInterpolatedBalloon() };
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
