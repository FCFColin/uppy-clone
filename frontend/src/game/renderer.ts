import { getState } from './store.js';
import { commitRenderedState } from './state_interp.js';
import { $canvas, getCtx, resizeCanvas as resizeCanvasBase } from './renderer_canvas.js';
import { drawBackground, invalidateBackgroundStaticCache } from './renderer_background.js';
import { drawBalloon, drawBird, drawGhost, drawRipples, drawExplosion } from './renderer_draw.js';
import {
  drawTutorialRangeCircle, drawDangerVignettes, drawFloatingTexts,
} from './visual_helpers.js';
import { drainPendingMessages } from './ws_message_queue.js';

export { $canvas } from './renderer_canvas.js';

export function resizeCanvas(): void {
  resizeCanvasBase();
  invalidateBackgroundStaticCache();
}

let renderActive = true;

export function setRenderActive(active: boolean): void {
  renderActive = active;
}

export function renderOnce(): void {
  render();
}

let _previousTimestamp: number | undefined;

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
  drainPendingMessages(8);
  render();
  requestAnimationFrame(gameLoop);
}

function render(): void {
  try {
    const now = Date.now();
    getCtx().fillStyle = '#1a1a2e';
    getCtx().fillRect(0, 0, $canvas.width, $canvas.height);

    drawBackground(now);

    if (getState().blockGameRender) {
      return;
    }

    if (getState().phase === 'playing' || getState().phase === 'ended') {
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
      const playerMap = new Map(getState().players.map(p => [p.playerIndex, p]));
      drawRipples(now, playerMap);
      drawFloatingTexts(now);
      drawExplosion(now);
    }
  } catch (err: unknown) {
    console.error('Render error:', err);
  }
}
