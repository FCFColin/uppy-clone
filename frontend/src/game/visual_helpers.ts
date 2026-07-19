import { PHYSICS } from '../shared/game/constants.js';
import { $canvas, getCtx } from './renderer.js';
import { getState } from './state.js';
import { getInterpolatedBalloon, getInterpolatedBird } from './state_interp.js';
import { isRangeCircleVisible } from './tutorial.js';
import { registerResetFn } from './reset_registry.js';

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

export function drawTutorialRangeCircle(now: number = Date.now()): void {
  if (!isRangeCircleVisible()) return;
  const interp = getInterpolatedBalloon(now);
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

let _vignetteGradLeft: CanvasGradient | null = null;
let _vignetteGradRight: CanvasGradient | null = null;
let _vignetteCachedW = 0;

function _ensureVignetteGradients(ctx: CanvasRenderingContext2D): void {
  if (!_vignetteGradLeft) {
    _vignetteGradLeft = ctx.createLinearGradient(0, 0, 8, 0);
    _vignetteGradLeft.addColorStop(0, 'rgba(233, 69, 96, 0.18)');
    _vignetteGradLeft.addColorStop(1, 'rgba(233, 69, 96, 0)');
  }
  const w = $canvas.width;
  if (!_vignetteGradRight || _vignetteCachedW !== w) {
    _vignetteCachedW = w;
    _vignetteGradRight = ctx.createLinearGradient(w, 0, w - 8, 0);
    _vignetteGradRight.addColorStop(0, 'rgba(233, 69, 96, 0.18)');
    _vignetteGradRight.addColorStop(1, 'rgba(233, 69, 96, 0)');
  }
}

export function drawDangerVignettes(now: number): void {
  if (getState().phase !== 'playing') return;

  _ensureVignetteGradients(getCtx());

  const bird = getInterpolatedBird(now);
  if (bird?.active) {
    const edge = bird.x < 0.5 ? 'left' : 'right';
    getCtx().fillStyle = edge === 'left' ? _vignetteGradLeft! : _vignetteGradRight!;
    getCtx().fillRect(edge === 'left' ? 0 : $canvas.width - 8, 0, 8, $canvas.height);
  }
}

export function drawFloatingTexts(now: number): void {
  pruneFloatingTexts(now);
  getCtx().font = '13px system-ui, sans-serif';
  getCtx().textAlign = 'center';
  for (const ft of floatingTexts) {
    const age = now - ft.start;
    const alpha = 1 - age / 1500;
    getCtx().globalAlpha = alpha * 0.9;
    getCtx().fillStyle = '#ccc';
    getCtx().fillText(ft.text, ft.x * $canvas.width, (1 - ft.y) * $canvas.height - 20);
  }
  getCtx().globalAlpha = 1;
}

function pruneFloatingTexts(now: number): void {
  for (let i = floatingTexts.length - 1; i >= 0; i--) {
    if (now - floatingTexts[i]!.start > 1500) {
      floatingTexts.splice(i, 1);
    }
  }
}

// ─── Common drawing primitives ────────────────────────────────────────

/** Fill a circle with a solid color. */
export function fillCircle(ctx: CanvasRenderingContext2D, x: number, y: number, r: number, color: string): void {
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fillStyle = color;
  ctx.fill();
}

/** Draw an image with the given alpha, restoring globalAlpha to 1 afterwards. */
export function drawImageAlpha(
  ctx: CanvasRenderingContext2D,
  img: CanvasImageSource,
  x: number, y: number, w: number, h: number,
  alpha: number,
): void {
  ctx.globalAlpha = alpha;
  ctx.drawImage(img, x, y, w, h);
  ctx.globalAlpha = 1;
}

/** Draw a radial-gradient glow circle centered at (x, y). */
export function drawRadialGlow(
  ctx: CanvasRenderingContext2D,
  x: number, y: number, r: number,
  innerColor: string, outerColor: string,
): void {
  const grad = ctx.createRadialGradient(x, y, 0, x, y, r);
  grad.addColorStop(0, innerColor);
  grad.addColorStop(1, outerColor);
  ctx.fillStyle = grad;
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fill();
}

export function isLowHeightDanger(): boolean {
  return getState().phase === 'playing' && getState().balloon.y < 0.15;
}

/** Reset visual helpers state for a new game session. */
export function resetVisualHelpers(): void {
  floatingTexts.length = 0;
  lastFloatingAt = 0;
  _vignetteGradLeft = null;
  _vignetteGradRight = null;
  _vignetteCachedW = 0;
}

registerResetFn(resetVisualHelpers);
