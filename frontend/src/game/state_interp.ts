import { PHYSICS } from '../shared/game/constants.js';
import { MAX_SEEN_SEQS } from './constants.js';
import { dispatch, getState } from './state.js';
import { clearOutboundQueue, resetReconnectAttempts } from './ws_connection.js';
import { resetTutorial } from './tutorial.js';
import { resetWindHint, stopCooldownUpdater } from './ui_common.js';
import { resetUIUpdateCache } from './ui_update.js';
import { resetEntryFlowState } from './entry_flow.js';
import { clearRestartCountdownTimer } from './restart_vote_ui.js';
import { runRegisteredResets } from './reset_registry.js';

export const TICK_MS = PHYSICS.TICK_INTERVAL;
export const INTERP_DELAY_MS = PHYSICS.INTERP_DELAY_MS;
export const MAX_SNAPSHOT_BUFFER = 12;
export const TELEPORT_THRESHOLD = 0.05;

export interface InterpPoint {
  x: number;
  y: number;
}

export interface InterpGhostPoint {
  x: number;
  y: number;
  active: boolean;
}

export interface InterpBirdPoint {
  x: number;
  y: number;
  active: boolean;
}

export interface BalloonAnchor {
  tick: number;
  receivedAt: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

export interface EntityAnchor {
  tick: number;
  receivedAt: number;
  x: number;
  y: number;
  active: boolean;
}

function capBuffer<T>(buf: T[], max: number): void {
  while (buf.length > max) buf.shift();
}

function pushAnchor<T>(buffer: T[], item: T): void {
  buffer.push(item);
  capBuffer(buffer, MAX_SNAPSHOT_BUFFER);
}

export const balloonBuffer: BalloonAnchor[] = [];
export const ghostBuffer: EntityAnchor[] = [];
export const birdBuffer: EntityAnchor[] = [];

export function getRenderTime(): number {
  return Date.now() - INTERP_DELAY_MS;
}

export function pushAnchors(tickCount: number): void {
  const receivedAt = Date.now();
  const s = getState();
  pushAnchor(balloonBuffer, {
    tick: tickCount,
    receivedAt,
    x: s.balloon.x,
    y: s.balloon.y,
    vx: s.balloon.vx,
    vy: s.balloon.vy,
  });
  pushAnchor(ghostBuffer, { tick: tickCount, receivedAt, x: s.ghost.x, y: s.ghost.y, active: s.ghost.active });
  pushAnchor(birdBuffer, { tick: tickCount, receivedAt, x: s.bird.x, y: s.bird.y, active: s.bird.active });
}

export function clearAnchorBuffers(): void {
  balloonBuffer.length = 0;
  ghostBuffer.length = 0;
  birdBuffer.length = 0;
}

export function findAnchorIndex(buffer: { receivedAt: number }[], renderTime: number): number {
  let index = -1;
  for (let i = 0; i < buffer.length; i++) {
    if (buffer[i]!.receivedAt <= renderTime) index = i;
  }
  return index;
}

export function tryBalloonFromDelayBuffer(): InterpPoint | null {
  if (balloonBuffer.length === 0) return null;
  const renderTime = getRenderTime();
  const i = findAnchorIndex(balloonBuffer, renderTime);
  if (i < 0) return null;

  const a = balloonBuffer[i]!;
  const b = balloonBuffer[i + 1];
  if (!b) {
    const ticks = (renderTime - a.receivedAt) / TICK_MS;
    return { x: a.x + a.vx * ticks, y: a.y + a.vy * ticks - 0.5 * PHYSICS.GRAVITY * ticks * ticks };
  }

  const span = b.receivedAt - a.receivedAt;
  const t = Math.max(0, Math.min(1, (renderTime - a.receivedAt) / span));
  return {
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
  };
}

export function tryEntityFromDelayBuffer(buffer: EntityAnchor[], renderTime: number): InterpPoint | null {
  if (buffer.length === 0) return null;
  const i = findAnchorIndex(buffer, renderTime);
  if (i < 0) return null;

  const a = buffer[i]!;
  const b = buffer[i + 1];
  if (!a.active) return null;
  if (!b) return { x: a.x, y: a.y };
  if (!b.active) return { x: a.x, y: a.y };

  const span = b.receivedAt - a.receivedAt;
  const t = Math.max(0, Math.min(1, (renderTime - a.receivedAt) / span));
  return {
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
  };
}

let seenSeqs: Set<number> = new Set();
// game-013: Track the highest seq seen to detect uint32 wrap-around.
let maxSeen = -1;
const WRAP_THRESHOLD = MAX_SEEN_SEQS * 2;

export function isDuplicateSeq(seq: number): boolean {
  if (maxSeen >= 0 && seq < maxSeen - WRAP_THRESHOLD) {
    seenSeqs = new Set();
    maxSeen = seq;
  }

  if (seenSeqs.has(seq)) return true;
  seenSeqs.add(seq);
  if (seq > maxSeen) {
    maxSeen = seq;
  }
  if (seenSeqs.size > MAX_SEEN_SEQS) {
    const entries = [...seenSeqs];
    seenSeqs = new Set(entries.slice(Math.floor(MAX_SEEN_SEQS / 2)));
  }
  return false;
}

export function clearSeenSeqs(): void {
  seenSeqs.clear();
  maxSeen = -1;
}

export function getSeenSeqsSize(): number {
  return seenSeqs.size;
}

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

function getInterpolatedEntityPrevCurr(curr: ActivePoint, prev: ActivePoint | null, now: number): ActivePoint | null {
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
  resetTutorial();
  resetWindHint();
  resetReconnectAttempts();
  resetUIUpdateCache();
  resetEntryFlowState();
  stopCooldownUpdater();
  clearRestartCountdownTimer();
  runRegisteredResets();
}
