import { PHYSICS } from '../shared/game/constants.js';
import { $canvas, getCtx } from './renderer_canvas.js';
import { getState } from './store.js';
import { getInterpolatedBalloon, getInterpolatedBird, getInterpolatedGhost } from './state_interp.js';
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
  mutateFloatingTexts(ft => { ft.push({ x, y, text, start: now }); });
}

function mutateFloatingTexts(mutate: (arr: FloatingText[]) => void): void {
  mutate(floatingTexts);
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

  const ghost = getInterpolatedGhost(now);
  if (ghost && ghost.active) {
    const balloon = getInterpolatedBalloon(now);
    const dx = ghost.x - balloon.x;
    const dy = ghost.y - balloon.y;
    const dist = Math.hypot(dx, dy);
    if (dist < 0.12) {
      getCtx().globalAlpha = 0.85 + 0.15 * Math.sin(now * 0.008);
    }
  }
  getCtx().globalAlpha = 1;
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
  mutateFloatingTexts(arr => {
    for (let i = arr.length - 1; i >= 0; i--) {
      if (now - arr[i]!.start > 1500) {
        arr.splice(i, 1);
      }
    }
  });
}

export function isLowHeightDanger(): boolean {
  return getState().phase === 'playing' && getState().balloon.y < 0.15;
}
