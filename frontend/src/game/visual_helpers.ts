import { PHYSICS } from '../shared/game/constants.js';
import { $canvas, getCtx } from './renderer.js';
import { getState } from './state.js';
import { getInterpolatedBalloon, getInterpolatedBird, getInterpolatedGhost } from './state_interp.js';
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
  const ctx = getCtx();
  const pulse = 0.8 + Math.sin(now * 0.003) * 0.2;

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  const glowGrad = ctx.createRadialGradient(bx, by, radius * 0.5, bx, by, radius * 1.3);
  glowGrad.addColorStop(0, `rgba(140, 100, 220, ${0.12 * pulse})`);
  glowGrad.addColorStop(0.5, `rgba(120, 80, 200, ${0.08 * pulse})`);
  glowGrad.addColorStop(1, 'rgba(100, 60, 180, 0)');
  ctx.fillStyle = glowGrad;
  ctx.beginPath();
  ctx.arc(bx, by, radius * 1.3, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  ctx.beginPath();
  ctx.arc(bx, by, radius, 0, Math.PI * 2);
  ctx.setLineDash([8, 10]);
  ctx.strokeStyle = `rgba(160, 130, 255, ${0.35 * pulse})`;
  ctx.lineWidth = 2;
  ctx.shadowBlur = 12;
  ctx.shadowColor = 'rgba(140, 100, 230, 0.4)';
  ctx.stroke();
  ctx.shadowBlur = 0;
  ctx.setLineDash([]);
}

// ─── 碰撞框调试可视化 ────────────────────────────────────────────────

/** 返回 true 当 URL 包含 ?debug=collision */
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

/**
 * 描边有效碰撞体积：
 * - 鸟椭圆: rx = BIRD_COLLISION_RADIUS_X + BALLOON_COLLISION_RADIUS, ry = BIRD_COLLISION_RADIUS_Y + BALLOON_COLLISION_RADIUS
 * - 鬼椭圆: rx = GHOST_COLLISION_RADIUS_X + BALLOON_COLLISION_RADIUS, ry = GHOST_COLLISION_RADIUS_Y + BALLOON_COLLISION_RADIUS
 * - 气球圆: r = BALLOON_COLLISION_RADIUS
 *
 * 输入使用归一化 [0,1] 坐标，按 min(canvas.width, canvas.height) 缩放为像素。
 * 不活跃的鸟/鬼跳过绘制（已离开屏幕）。
 */
export function drawCollisionDebug(
  ctx: CanvasRenderingContext2D,
  bird: CollisionEntity | null,
  ghost: CollisionEntity | null,
  balloon: CollisionBalloon,
): void {
  const scale = Math.min($canvas.width, $canvas.height);
  const bx = balloon.x * $canvas.width;
  const by = (1 - balloon.y) * $canvas.height;
  const balloonR = PHYSICS.BALLOON_COLLISION_RADIUS * scale;

  ctx.save();
  ctx.strokeStyle = '#ff0000';
  ctx.lineWidth = 1.5;

  // 气球碰撞圆
  ctx.beginPath();
  ctx.arc(bx, by, balloonR, 0, Math.PI * 2);
  ctx.stroke();

  // 鸟有效碰撞椭圆
  if (bird && bird.active) {
    const birdRx = (PHYSICS.BIRD_COLLISION_RADIUS_X + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const birdRy = (PHYSICS.BIRD_COLLISION_RADIUS_Y + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const cx = bird.x * $canvas.width;
    const cy = (1 - bird.y) * $canvas.height;
    ctx.beginPath();
    ctx.ellipse(cx, cy, birdRx, birdRy, 0, 0, Math.PI * 2);
    ctx.stroke();
  }

  // 鬼有效碰撞椭圆
  if (ghost && ghost.active) {
    const ghostRx = (PHYSICS.GHOST_COLLISION_RADIUS_X + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const ghostRy = (PHYSICS.GHOST_COLLISION_RADIUS_Y + PHYSICS.BALLOON_COLLISION_RADIUS) * scale;
    const cx = ghost.x * $canvas.width;
    const cy = (1 - ghost.y) * $canvas.height;
    ctx.beginPath();
    ctx.ellipse(cx, cy, ghostRx, ghostRy, 0, 0, Math.PI * 2);
    ctx.stroke();
  }

  ctx.restore();
}

let _vignetteGradLeft: CanvasGradient | null = null;
let _vignetteGradRight: CanvasGradient | null = null;
let _vignetteCachedW = 0;

function _ensureVignetteGradients(ctx: CanvasRenderingContext2D, now: number): void {
  const pulse = 0.85 + Math.sin(now * 0.005) * 0.15;
  const w = $canvas.width;
  _vignetteGradLeft = ctx.createLinearGradient(0, 0, 25, 0);
  _vignetteGradLeft.addColorStop(0, `rgba(100, 40, 160, ${0.35 * pulse})`);
  _vignetteGradLeft.addColorStop(0.3, `rgba(120, 60, 180, ${0.25 * pulse})`);
  _vignetteGradLeft.addColorStop(0.6, `rgba(80, 100, 200, ${0.12 * pulse})`);
  _vignetteGradLeft.addColorStop(1, 'rgba(60, 80, 180, 0)');
  _vignetteGradRight = ctx.createLinearGradient(w, 0, w - 25, 0);
  _vignetteGradRight.addColorStop(0, `rgba(100, 40, 160, ${0.35 * pulse})`);
  _vignetteGradRight.addColorStop(0.3, `rgba(120, 60, 180, ${0.25 * pulse})`);
  _vignetteGradRight.addColorStop(0.6, `rgba(80, 100, 200, ${0.12 * pulse})`);
  _vignetteGradRight.addColorStop(1, 'rgba(60, 80, 180, 0)');
  _vignetteCachedW = w;
}

export function drawDangerVignettes(now: number): void {
  if (getState().phase !== 'playing') return;

  const ctx = getCtx();
  _ensureVignetteGradients(ctx, now);

  const bird = getInterpolatedBird(now);
  if (bird?.active) {
    const edge = bird.x < 0.5 ? 'left' : 'right';
    const vw = 30;
    ctx.save();
    ctx.globalCompositeOperation = 'screen';
    ctx.fillStyle = edge === 'left' ? _vignetteGradLeft! : _vignetteGradRight!;
    ctx.fillRect(edge === 'left' ? 0 : $canvas.width - vw, 0, vw, $canvas.height);
    ctx.restore();
  }

  const ghost = getInterpolatedGhost(now);
  if (ghost) {
    const gx = ghost.x * $canvas.width;
    const gy = (1 - ghost.y) * $canvas.height;
    const distToCenter = Math.abs(gx - $canvas.width / 2);
    if (distToCenter < $canvas.width * 0.3) {
      const dangerIntensity = 1 - distToCenter / ($canvas.width * 0.3);
      const pulse = 0.7 + Math.sin(now * 0.008) * 0.3;
      ctx.save();
      ctx.globalCompositeOperation = 'screen';
      const grad = ctx.createRadialGradient(gx, gy, 0, gx, gy, Math.min($canvas.width, $canvas.height) * 0.15);
      grad.addColorStop(0, `rgba(140, 50, 180, ${dangerIntensity * 0.25 * pulse})`);
      grad.addColorStop(0.5, `rgba(100, 40, 160, ${dangerIntensity * 0.12 * pulse})`);
      grad.addColorStop(1, 'rgba(80, 30, 140, 0)');
      ctx.fillStyle = grad;
      ctx.beginPath();
      ctx.arc(gx, gy, Math.min($canvas.width, $canvas.height) * 0.15, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    }
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
  x: number,
  y: number,
  w: number,
  h: number,
  alpha: number,
): void {
  ctx.globalAlpha = alpha;
  ctx.drawImage(img, x, y, w, h);
  ctx.globalAlpha = 1;
}

/** Draw a radial-gradient glow circle centered at (x, y). */
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

/** Reset visual helpers state for a new game session. */
export function resetVisualHelpers(): void {
  floatingTexts.length = 0;
  lastFloatingAt = 0;
  _vignetteGradLeft = null;
  _vignetteGradRight = null;
  _vignetteCachedW = 0;
}

registerResetFn(resetVisualHelpers);
