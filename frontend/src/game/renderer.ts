import { getState } from './store.js';
import type { ClientPlayer } from './state_types.js';
import { commitRenderedState } from './state_interp.js';
import { $canvas, getCtx, resizeCanvas as resizeCanvasBase } from './renderer_canvas.js';
import { drawBackground, invalidateBackgroundStaticCache } from './renderer_background.js';
import { drawBalloon, drawBird, drawGhost, drawRipples, drawExplosion, pruneEffects } from './renderer_draw.js';
import {
  drawTutorialRangeCircle, drawDangerVignettes, drawFloatingTexts,
} from './visual_helpers.js';
import { drainPendingMessages } from './ws_message_queue.js';
import { registerResetFn } from './reset_registry.js';

export { $canvas } from './renderer_canvas.js';

export function resizeCanvas(): void {
  resizeCanvasBase();
  invalidateBackgroundStaticCache();
}

let renderActive = true;
let loopRunning = false;
let cachedPlayerMap: Map<number, ClientPlayer> | null = null;
let cachedPlayerMapKey: string | null = null;

export function setRenderActive(active: boolean): void {
  renderActive = active;
}

export function isLoopRunning(): boolean {
  return loopRunning;
}

function getPlayerMap(): Map<number, ClientPlayer> {
  const players = getState().players;
  const key = players.map(p =>
    `${p.playerIndex}:${p.scoreContribution}:${p.nickname}:${p.palette}`
  ).join('|');
  if (key === cachedPlayerMapKey && cachedPlayerMap !== null) {
    return cachedPlayerMap;
  }
  cachedPlayerMapKey = key;
  cachedPlayerMap = new Map(players.map(p => [p.playerIndex, p]));
  return cachedPlayerMap;
}

export function renderOnce(): void {
  render();
}

let _previousTimestamp: number | undefined;

export function startGameLoop(): void {
  if (loopRunning) return;
  loopRunning = true;
  requestAnimationFrame(gameLoop);
}

export function gameLoop(timestamp: number): void {
  if (!renderActive) {
    requestAnimationFrame(gameLoop);
    return;
  }
  if (_previousTimestamp !== undefined) {
    const delta = timestamp - _previousTimestamp;
    if (delta > 33) {
      console.warn(`Frame budget exceeded: ${delta.toFixed(1)}ms (target: 16.7ms for 60fps)`);
    }
  }
  _previousTimestamp = timestamp;
  try {
    drainPendingMessages(8);
  } catch (err: unknown) {
    console.error('drainPendingMessages error:', err);
  }
  render();
  requestAnimationFrame(gameLoop);
}

function render(): void {
  try {
    const now = Date.now();
    getCtx().fillStyle = '#1a1a2e';
    getCtx().fillRect(0, 0, $canvas.width, $canvas.height);

    drawBackground(now);

    if (getState().blockGameRender) return;
    if (getState().phase !== 'playing' && getState().phase !== 'ended') return;

    if (getState().hasReceivedFirstSnapshot) {
      drawTutorialRangeCircle(now);
      drawBalloon(now);
      drawBird(now);
      drawGhost(now);
      drawDangerVignettes(now);
      if (getState().phase === 'playing') {
        commitRenderedState(now);
      }
    }

    const playerMap = getPlayerMap();
    pruneEffects();
    drawRipples(now, playerMap);
    drawFloatingTexts(now);
    drawExplosion(now);
  } catch (err: unknown) {
    console.error('Render error:', err);
  }
}

/** Reset renderer cache for a new game session. */
export function resetRendererState(): void {
  cachedPlayerMap = null;
  cachedPlayerMapKey = null;
  invalidateBackgroundStaticCache();
}

registerResetFn(resetRendererState);
