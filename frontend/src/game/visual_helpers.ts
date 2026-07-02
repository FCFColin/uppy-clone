import { PHYSICS } from './constants.js';
import { $canvas, getCtx } from './renderer_canvas.js';
import { state, getInterpolatedBalloon, getInterpolatedBird, getInterpolatedGhost } from './state.js';
import { isRangeCircleVisible } from './tutorial.js';

interface FloatingText {
  x: number;
  y: number;
  text: string;
  start: number;
}

const floatingTexts: FloatingText[] = [];
let lastFloatingAt = 0;

export function pushFloatingText(x: number, y: number, text: string): void {
  const now = Date.now();
  if (now - lastFloatingAt < 3000) return;
  lastFloatingAt = now;
  floatingTexts.push({ x, y, text, start: now });
}

export function drawTutorialRangeCircle(): void {
  if (!isRangeCircleVisible()) return;
  const interp = getInterpolatedBalloon();
  const bx = interp.x * $canvas.width;
  const by = (1 - interp.y) * $canvas.height;
  const radius = PHYSICS.TAP_RANGE * Math.min($canvas.width, $canvas.height);
  getCtx().beginPath();
  getCtx().arc(bx, by, radius, 0, Math.PI * 2);
  getCtx().setLineDash([6, 8]);
  getCtx().strokeStyle = 'rgba(168, 212, 255, 0.22)';
  getCtx().lineWidth = 1.5;
  getCtx().stroke();
  getCtx().setLineDash([]);
}

export function drawDangerVignettes(): void {
  if (state.phase !== 'playing') return;

  const bird = getInterpolatedBird();
  if (bird?.active) {
    const edge = bird.x < 0.5 ? 'left' : 'right';
    const grad = edge === 'left'
      ? getCtx().createLinearGradient(0, 0, 8, 0)
      : getCtx().createLinearGradient($canvas.width, 0, $canvas.width - 8, 0);
    grad.addColorStop(0, 'rgba(233, 69, 96, 0.18)');
    grad.addColorStop(1, 'rgba(233, 69, 96, 0)');
    getCtx().fillStyle = grad;
    getCtx().fillRect(edge === 'left' ? 0 : $canvas.width - 8, 0, 8, $canvas.height);
  }

  const ghost = getInterpolatedGhost();
  if (ghost && ghost.active) {
    const balloon = getInterpolatedBalloon();
    const dx = ghost.x - balloon.x;
    const dy = ghost.y - balloon.y;
    const dist = Math.hypot(dx, dy);
    if (dist < 0.12) {
      getCtx().globalAlpha = 0.85 + 0.15 * Math.sin(Date.now() * 0.008);
    }
  }
  getCtx().globalAlpha = 1;
}

export function drawFloatingTexts(): void {
  const now = Date.now();
  for (let i = floatingTexts.length - 1; i >= 0; i--) {
    const ft = floatingTexts[i]!;
    const age = now - ft.start;
    if (age > 1500) {
      floatingTexts.splice(i, 1);
      continue;
    }
    const alpha = 1 - age / 1500;
    getCtx().fillStyle = `rgba(204, 204, 204, ${alpha * 0.9})`;
    getCtx().font = '13px system-ui, sans-serif';
    getCtx().textAlign = 'center';
    getCtx().fillText(ft.text, ft.x * $canvas.width, (1 - ft.y) * $canvas.height - 20);
  }
}

export function isLowHeightDanger(): boolean {
  return state.phase === 'playing' && state.balloon.y < 0.15;
}
