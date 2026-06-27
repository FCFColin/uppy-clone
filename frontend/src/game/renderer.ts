import { FIXED_STEP } from './constants.js';
import { state } from './state.js';
import { updateCooldownBar } from './ui.js';
import { $canvas, ctx, resizeCanvas } from './renderer_canvas.js';
import { drawBackground } from './renderer_background.js';
import { drawBalloon, drawBird, drawGhost, drawRipples, drawExplosion } from './renderer_draw.js';

export { $canvas, resizeCanvas } from './renderer_canvas.js';

let renderActive: boolean = true;
let lastFrameTime: number = 0;

export function setRenderActive(active: boolean): void {
  renderActive = active;
}

export function renderOnce(): void {
  render();
}

export function gameLoop(timestamp: number): void {
  if (!renderActive) return;
  const delta: number = timestamp - lastFrameTime;
  if (delta >= FIXED_STEP) {
    render();
    lastFrameTime = timestamp;
  }
  requestAnimationFrame(gameLoop);
}

function overlayBlocksGameRender(): boolean {
  const endedScreen: HTMLElement | null = document.getElementById('ended-screen');
  const nickSetup: HTMLElement | null = document.getElementById('nickname-setup-screen');
  const waitingScreen: HTMLElement | null = document.getElementById('waiting-screen');
  if (endedScreen && !endedScreen.classList.contains('hidden')) return true;
  if (nickSetup && !nickSetup.classList.contains('hidden') && !state.nicknameSubmitted) return true;
  if (waitingScreen && !waitingScreen.classList.contains('hidden') && state.nicknameSubmitted && state.phase === 'waiting') {
    return true;
  }
  return false;
}

function render(): void {
  try {
    ctx.fillStyle = '#1a1a2e';
    ctx.fillRect(0, 0, $canvas.width, $canvas.height);

    if (overlayBlocksGameRender()) {
      return;
    }

    drawBackground();

    if (state.phase === 'playing' || state.phase === 'ended') {
      if (state.hasReceivedFirstSnapshot) {
        drawBalloon();
        if (state.bird.active) drawBird();
        drawGhost();
      }
      drawRipples();
      drawExplosion();
    }

    if (state.phase === 'playing') {
      updateCooldownBar();
    }
  } catch (err: unknown) {
    console.error('Render error:', err);
  }
}
