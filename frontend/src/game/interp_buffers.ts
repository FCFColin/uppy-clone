import { PHYSICS } from '../shared/game/constants.js';
import { getState } from './store.js';



export const TICK_MS: number = PHYSICS.TICK_INTERVAL;
export const INTERP_DELAY_MS: number = PHYSICS.INTERP_DELAY_MS;
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
  pushAnchor(balloonBuffer, { tick: tickCount, receivedAt, x: s.balloon.x, y: s.balloon.y, vx: s.balloon.vx, vy: s.balloon.vy });
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

export function tryEntityFromDelayBuffer(
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
  const t = Math.max(0, Math.min(1, (renderTime - a.receivedAt) / span));
  return {
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
  };
}
