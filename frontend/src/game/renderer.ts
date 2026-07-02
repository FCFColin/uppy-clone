import { state, commitRenderedState } from './state.js';
import { $endedScreen, $nicknameSetupScreen, $waitingScreen } from './ui_elements.js';
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

export function gameLoop(_timestamp: number): void {
  if (!renderActive) {
    requestAnimationFrame(gameLoop);
    return;
  }
  drainPendingMessages(8);
  render();
  requestAnimationFrame(gameLoop);
}

function overlayBlocksGameRender(): boolean {
  if ($endedScreen && !$endedScreen.classList.contains('hidden')) return true;
  if ($nicknameSetupScreen && !$nicknameSetupScreen.classList.contains('hidden') && !state.nicknameSubmitted) return true;
  if ($waitingScreen && !$waitingScreen.classList.contains('hidden') && state.nicknameSubmitted && state.phase === 'waiting') {
    return true;
  }
  const tutorial = document.getElementById('tutorial-overlay');
  if (tutorial && !tutorial.classList.contains('hidden')) return true;
  return false;
}

function render(): void {
  try {
    const now = Date.now();
    getCtx().fillStyle = '#1a1a2e';
    getCtx().fillRect(0, 0, $canvas.width, $canvas.height);

    drawBackground(now);

    if (overlayBlocksGameRender()) {
      return;
    }

    if (state.phase === 'playing' || state.phase === 'ended') {
      if (state.hasReceivedFirstSnapshot) {
        drawTutorialRangeCircle(now);
        drawBalloon(now);
        drawBird(now);
        drawGhost(now);
        drawDangerVignettes(now);
        if (state.phase === 'playing') {
          commitRenderedState(now);
        }
      }
      const playerMap = new Map(state.players.map(p => [p.playerIndex, p]));
      drawRipples(now, playerMap);
      drawFloatingTexts(now);
      drawExplosion(now);
    }
  } catch (err: unknown) {
    console.error('Render error:', err);
  }
}
