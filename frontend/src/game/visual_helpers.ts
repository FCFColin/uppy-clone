import { PHYSICS } from '../shared/game/constants.js';
import { getCtx, getCssCanvasSize } from './renderer.js';
import { getState } from './state.js';
import { getInterpolatedBalloon, getInterpolatedBird, getInterpolatedGhost } from './state_interp.js';
import { registerResetFn } from './reset_registry.js';
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

export function drawTutorialRangeCircle(now: number = Date.now()): void {
  if (!isRangeCircleVisible()) return;
  const cssSize = getCssCanvasSize();
  const interp = getInterpolatedBalloon(now);
  const bx = interp.x * cssSize.width;
  const by = (1 - interp.y) * cssSize.height;
  const radius = PHYSICS.TAP_RANGE * Math.min(cssSize.width, cssSize.height);
  const ctx = getCtx();
  const reduced = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const pulse = reduced ? 1 : 0.82 + Math.sin(now * 0.003) * 0.18;
  const rotate = reduced ? 0 : (now * 0.0003) % (Math.PI * 2);
  _ensureRangeGradients(ctx, bx, by, radius);

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  ctx.globalAlpha = pulse;
  ctx.fillStyle = _rangeFillGrad!;
  ctx.beginPath();
  ctx.arc(bx, by, radius * 1.25, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = _rangeGlowGrad!;
  ctx.beginPath();
  ctx.arc(bx, by, radius * 1.45, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  ctx.save();
  ctx.translate(bx, by);
  ctx.rotate(rotate);
  ctx.translate(-bx, -by);
  ctx.beginPath();
  ctx.arc(bx, by, radius, 0, Math.PI * 2);
  ctx.setLineDash([10, 12]);
  ctx.strokeStyle = `rgba(170, 140, 255, ${0.45 * pulse})`;
  ctx.lineWidth = 2.5;
  ctx.lineCap = 'round';
  ctx.shadowBlur = 14;
  ctx.shadowColor = `rgba(140, 100, 230, ${0.5 * pulse})`;
  ctx.stroke();
  ctx.setLineDash([]);
  ctx.restore();

  if (!reduced) {
    ctx.save();
    ctx.translate(bx, by);
    ctx.rotate(-rotate * 0.7 + Math.PI / 6);
    ctx.translate(-bx, -by);
    ctx.beginPath();
    ctx.arc(bx, by, radius * 0.92, 0, Math.PI * 2);
    ctx.setLineDash([6, 18]);
    ctx.strokeStyle = `rgba(190, 160, 255, ${0.25 * pulse})`;
    ctx.lineWidth = 1.5;
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.restore();
  }

  ctx.shadowBlur = 0;
}

export function isCollisionDebugEnabled(): boolean {
  try {
    const params = new URLSearchParams(globalThis.location.search);
    return params.get('debug') === 'collision';
  } catch {
    return false;
  }
}

interface CollisionEntity {
  x: number;
  y: number;
  active: boolean;
}

interface CollisionBalloon {
  x: number;
  y: number;
}

export function drawCollisionDebug(
  ctx: CanvasRenderingContext2D,
  bird: CollisionEntity | null,
  ghost: CollisionEntity | null,
  balloon: CollisionBalloon,
): void {
  const cssSize = getCssCanvasSize();
  const scale = Math.min(cssSize.width, cssSize.height);
  const bx = balloon.x * cssSize.width;
  const by = (1 - balloon.y) * cssSize.height;
  const balloonR = PHYSICS.BALLOON_COLLISION_RADIUS * scale;

  ctx.save();
  ctx.strokeStyle = '#ff0000';
  ctx.lineWidth = 1.5;

  ctx.beginPath();
  ctx.arc(bx, by, balloonR, 0, Math.PI * 2);
  ctx.stroke();

  if (bird && bird.active) {
    const birdRx = (PHYSICS.BIRD_COLLISION_RADIUS_X + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const birdRy = (PHYSICS.BIRD_COLLISION_RADIUS_Y + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const cx = bird.x * cssSize.width;
    const cy = (1 - bird.y) * cssSize.height;
    ctx.beginPath();
    ctx.ellipse(cx, cy, birdRx, birdRy, 0, 0, Math.PI * 2);
    ctx.stroke();
  }

  if (ghost && ghost.active) {
    const ghostRx = (PHYSICS.GHOST_COLLISION_RADIUS_X + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const ghostRy = (PHYSICS.GHOST_COLLISION_RADIUS_Y + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const cx = ghost.x * cssSize.width;
    const cy = (1 - ghost.y) * cssSize.height;
    ctx.beginPath();
    ctx.ellipse(cx, cy, ghostRx, ghostRy, 0, 0, Math.PI * 2);
    ctx.stroke();
  }

  ctx.restore();
}

let _vignetteGradLeft: CanvasGradient | null = null;
let _vignetteGradRight: CanvasGradient | null = null;
let _vignetteCachedW = 0;
let _vignetteCachedH = 0;

let _cachedRangeRadius = 0;
let _rangeFillGrad: CanvasGradient | null = null;
let _rangeGlowGrad: CanvasGradient | null = null;

function _ensureRangeGradients(ctx: CanvasRenderingContext2D, bx: number, by: number, radius: number): void {
  if (_cachedRangeRadius === radius && _rangeFillGrad && _rangeGlowGrad) return;
  _cachedRangeRadius = radius;
  _rangeFillGrad = ctx.createRadialGradient(bx, by, 0, bx, by, radius * 1.25);
  _rangeFillGrad.addColorStop(0, 'rgba(160, 130, 255, 0.06)');
  _rangeFillGrad.addColorStop(0.6, 'rgba(140, 110, 230, 0.04)');
  _rangeFillGrad.addColorStop(1, 'rgba(120, 90, 210, 0)');

  _rangeGlowGrad = ctx.createRadialGradient(bx, by, radius * 0.85, bx, by, radius * 1.45);
  _rangeGlowGrad.addColorStop(0, 'rgba(160, 120, 255, 0.18)');
  _rangeGlowGrad.addColorStop(0.5, 'rgba(140, 100, 230, 0.1)');
  _rangeGlowGrad.addColorStop(1, 'rgba(120, 80, 210, 0)');
}

function _ensureVignetteGradients(ctx: CanvasRenderingContext2D, now: number): void {
  const cssSize = getCssCanvasSize();
  const reduced = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const pulse = reduced ? 1 : 0.82 + Math.sin(now * 0.005) * 0.18;
  const w = cssSize.width;
  const h = cssSize.height;
  const vw = Math.min(w * 0.08, 90);
  _vignetteGradLeft = ctx.createLinearGradient(0, 0, vw, 0);
  _vignetteGradLeft.addColorStop(0, `rgba(120, 40, 170, ${0.42 * pulse})`);
  _vignetteGradLeft.addColorStop(0.25, `rgba(130, 60, 190, ${0.28 * pulse})`);
  _vignetteGradLeft.addColorStop(0.55, `rgba(100, 70, 190, ${0.14 * pulse})`);
  _vignetteGradLeft.addColorStop(1, 'rgba(70, 60, 160, 0)');
  _vignetteGradRight = ctx.createLinearGradient(w, 0, w - vw, 0);
  _vignetteGradRight.addColorStop(0, `rgba(120, 40, 170, ${0.42 * pulse})`);
  _vignetteGradRight.addColorStop(0.25, `rgba(130, 60, 190, ${0.28 * pulse})`);
  _vignetteGradRight.addColorStop(0.55, `rgba(100, 70, 190, ${0.14 * pulse})`);
  _vignetteGradRight.addColorStop(1, 'rgba(70, 60, 160, 0)');
  _vignetteCachedW = w;
  _vignetteCachedH = h;
}

export function drawDangerVignettes(now: number): void {
  if (getState().phase !== 'playing') return;

  const ctx = getCtx();
  const cssSize = getCssCanvasSize();
  const w = cssSize.width;
  const h = cssSize.height;
  if (_vignetteCachedW !== w || _vignetteCachedH !== h) {
    _ensureVignetteGradients(ctx, now);
  }

  const bird = getInterpolatedBird(now);
  if (bird?.active) {
    const edge = bird.x < 0.5 ? 'left' : 'right';
    const vw = Math.min(w * 0.08, 90);
    ctx.save();
    ctx.globalCompositeOperation = 'screen';
    ctx.fillStyle = edge === 'left' ? _vignetteGradLeft! : _vignetteGradRight!;
    ctx.fillRect(edge === 'left' ? 0 : w - vw, 0, vw, h);
    ctx.globalAlpha = 1;
    ctx.fillStyle = 'rgba(80, 40, 120, 0.08)';
    ctx.fillRect(0, 0, w, h * 0.06);
    ctx.restore();
  }

  const ghost = getInterpolatedGhost(now);
  if (ghost) {
    const gx = ghost.x * w;
    const gy = (1 - ghost.y) * h;
    const distToCenter = Math.abs(gx - w / 2);
    const dangerZone = w * 0.2;
    if (distToCenter < dangerZone) {
      const dangerIntensity = 1 - distToCenter / dangerZone;
      const radius = Math.min(w, h) * 0.08;
      ctx.save();
      ctx.globalCompositeOperation = 'screen';
      ctx.globalAlpha = dangerIntensity * 0.12;
      ctx.fillStyle = 'rgba(200, 180, 230, 1)';
      ctx.beginPath();
      ctx.arc(gx, gy, radius, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
  }
}

export function drawFloatingTexts(now: number): void {
  pruneFloatingTexts(now);
  const cssSize = getCssCanvasSize();
  getCtx().font = '13px system-ui, sans-serif';
  getCtx().textAlign = 'center';
  for (const ft of floatingTexts) {
    const age = now - ft.start;
    const alpha = 1 - age / 1500;
    getCtx().globalAlpha = alpha * 0.9;
    getCtx().fillStyle = '#ccc';
    getCtx().fillText(ft.text, ft.x * cssSize.width, (1 - ft.y) * cssSize.height - 20);
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

export function fillCircle(ctx: CanvasRenderingContext2D, x: number, y: number, r: number, color: string): void {
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fillStyle = color;
  ctx.fill();
}

export function drawRadialGlow(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  r: number,
  innerColor: string,
  outerColor: string,
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

export function resetVisualHelpers(): void {
  floatingTexts.length = 0;
  lastFloatingAt = 0;
  _vignetteGradLeft = null;
  _vignetteGradRight = null;
  _vignetteCachedW = 0;
  _vignetteCachedH = 0;
  _cachedRangeRadius = 0;
  _rangeFillGrad = null;
  _rangeGlowGrad = null;
}

registerResetFn(resetVisualHelpers);
