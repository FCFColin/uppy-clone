import { $canvas, getCtx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { dispatch, getState } from './store.js';
import { getInterpolatedBalloon, getInterpolatedGhost, getInterpolatedBird } from './state_interp.js';
import { PALETTE_COLORS } from '../shared/game/constants.js';

let _cachedBirdSize = 0;
let _wingGrad: CanvasGradient | null = null;
let _bodyGrad: CanvasGradient | null = null;

function _ensureBirdGradients(ctx: CanvasRenderingContext2D, size: number): void {
  if (_cachedBirdSize === size && _wingGrad && _bodyGrad) return;
  _cachedBirdSize = size;
  _wingGrad = ctx.createLinearGradient(0, -size * 0.4, 0, size * 0.2);
  _wingGrad.addColorStop(0, '#e85d04');
  _wingGrad.addColorStop(1, '#dc2f02');
  _bodyGrad = ctx.createRadialGradient(-size * 0.15, -size * 0.15, size * 0.05, 0, 0, size * 0.6);
  _bodyGrad.addColorStop(0, '#ffba08');
  _bodyGrad.addColorStop(0.6, '#f48c06');
  _bodyGrad.addColorStop(1, '#e85d04');
}

export function drawBalloon(now: number = Date.now()): void {
  const interp = getInterpolatedBalloon(now);
  const bx = interp.x * $canvas.width;
  const by = (1 - interp.y) * $canvas.height;
  const radius = Math.min($canvas.width, $canvas.height) * 0.06;

  const balloonImg = gameImages['balloon'];
  if (balloonImg && balloonImg.loaded) {
    const img: HTMLImageElement = balloonImg.img;
    const w = radius * 2.5;
    const h = w * (img.height / img.width);
    const tilt = Math.max(-5, Math.min(5, getState().wind * 40)) * (Math.PI / 180);
    getCtx().save();
    getCtx().translate(bx, by);
    getCtx().rotate(tilt);
    getCtx().drawImage(img, -w / 2, -h / 2, w, h);
    getCtx().restore();
    return;
  }

  getCtx().beginPath();
  getCtx().arc(bx, by, radius, 0, Math.PI * 2);
  const balloonGrad: CanvasGradient = getCtx().createRadialGradient(
    bx - radius * 0.3, by - radius * 0.3, radius * 0.1,
    bx, by, radius
  );
  balloonGrad.addColorStop(0, '#ff6b6b');
  balloonGrad.addColorStop(1, '#e94560');
  getCtx().fillStyle = balloonGrad;
  getCtx().fill();

  getCtx().beginPath();
  getCtx().arc(bx - radius * 0.25, by - radius * 0.25, radius * 0.2, 0, Math.PI * 2);
  getCtx().fillStyle = 'rgba(255,255,255,0.3)';
  getCtx().fill();

  getCtx().beginPath();
  getCtx().moveTo(bx, by + radius);
  getCtx().lineTo(bx, by + radius + radius * 0.8);
  getCtx().strokeStyle = '#aaa';
  getCtx().lineWidth = 2;
  getCtx().stroke();
}

function drawBirdTail(ctx: CanvasRenderingContext2D, size: number): void {
  ctx.beginPath();
  ctx.moveTo(-size * 0.5, 0);
  ctx.lineTo(-size * 0.85, -size * 0.2);
  ctx.lineTo(-size * 0.8, 0);
  ctx.lineTo(-size * 0.85, size * 0.2);
  ctx.closePath();
  ctx.fillStyle = '#dc2f02';
  ctx.fill();
}

function drawBirdBeak(ctx: CanvasRenderingContext2D, size: number): void {
  ctx.beginPath();
  ctx.moveTo(size * 0.55, -size * 0.05);
  ctx.lineTo(size * 0.8, 0);
  ctx.lineTo(size * 0.55, size * 0.1);
  ctx.closePath();
  ctx.fillStyle = '#ffba08';
  ctx.fill();
}

function drawBirdEyes(ctx: CanvasRenderingContext2D, size: number): void {
  ctx.beginPath();
  ctx.arc(size * 0.32, -size * 0.12, size * 0.1, 0, Math.PI * 2);
  ctx.fillStyle = '#fff';
  ctx.fill();
  ctx.beginPath();
  ctx.arc(size * 0.35, -size * 0.12, size * 0.05, 0, Math.PI * 2);
  ctx.fillStyle = '#000';
  ctx.fill();
}

export function drawBird(now: number): void {
  const bird = getInterpolatedBird(now);
  if (!bird || !bird.active) return;
  const bx: number = bird.x * $canvas.width;
  const by: number = (1 - bird.y) * $canvas.height;
  const size: number = Math.min($canvas.width, $canvas.height) * 0.035;

  const vx = getState().balloon.x - bird.x;
  const dir = vx >= 0 ? 1 : -1;

  const flapPhase = Math.sin(now * 0.012);
  const wingOffset = flapPhase * size * 0.5;

  _ensureBirdGradients(getCtx(), size);

  getCtx().save();
  getCtx().translate(bx, by);
  getCtx().scale(dir, 1);

  getCtx().beginPath();
  getCtx().ellipse(-size * 0.15, -wingOffset, size * 0.55, size * 0.28, -0.35, 0, Math.PI * 2);
  getCtx().fillStyle = _wingGrad!;
  getCtx().fill();

  getCtx().beginPath();
  getCtx().ellipse(0, 0, size * 0.6, size * 0.42, 0, 0, Math.PI * 2);
  getCtx().fillStyle = _bodyGrad!;
  getCtx().fill();

  getCtx().beginPath();
  getCtx().ellipse(size * 0.05, -wingOffset, size * 0.45, size * 0.22, 0.3, 0, Math.PI * 2);
  getCtx().fillStyle = _wingGrad!;
  getCtx().fill();

  drawBirdTail(getCtx(), size);
  drawBirdBeak(getCtx(), size);
  drawBirdEyes(getCtx(), size);

  getCtx().restore();
}

export function drawGhost(now: number): void {
  const interpGhost = getInterpolatedGhost(now);
  if (!interpGhost) return;
  const radius: number = Math.min($canvas.width, $canvas.height) * 0.035;
  const gx = interpGhost.x * $canvas.width;
  const gy = (1 - interpGhost.y) * $canvas.height;

  const isRepelled = getState().ghost.repelTimer > 0;
  const baseColor = isRepelled ? '255, 100, 100' : '180, 100, 255';

  const ghostImg = gameImages['ghost'];
  if (ghostImg && ghostImg.loaded) {
    const size: number = radius * 4;
    if (isRepelled) {
      const glowGrad: CanvasGradient = getCtx().createRadialGradient(gx, gy, 0, gx, gy, size * 0.7);
      glowGrad.addColorStop(0, 'rgba(255, 50, 50, 0.6)');
      glowGrad.addColorStop(1, 'rgba(255, 50, 50, 0)');
      getCtx().fillStyle = glowGrad;
      getCtx().beginPath();
      getCtx().arc(gx, gy, size * 0.7, 0, Math.PI * 2);
      getCtx().fill();
      const flash: boolean = Math.sin(now * 0.02) > 0;
      getCtx().globalAlpha = flash ? 0.6 : 1;
    }
    getCtx().drawImage(ghostImg.img, gx - size / 2, gy - size / 2, size, size);
    getCtx().globalAlpha = 1;
    return;
  }

  const glowGrad: CanvasGradient = getCtx().createRadialGradient(gx, gy, 0, gx, gy, radius * 2);
  glowGrad.addColorStop(0, `rgba(${baseColor}, 0.4)`);
  glowGrad.addColorStop(1, `rgba(${baseColor}, 0)`);
  getCtx().fillStyle = glowGrad;
  getCtx().beginPath();
  getCtx().arc(gx, gy, radius * 2, 0, Math.PI * 2);
  getCtx().fill();

  const bodyGrad: CanvasGradient = getCtx().createRadialGradient(gx - radius * 0.3, gy - radius * 0.3, 0, gx, gy, radius);
  bodyGrad.addColorStop(0, `rgba(${baseColor}, 0.9)`);
  bodyGrad.addColorStop(1, `rgba(${baseColor}, 0.5)`);
  getCtx().fillStyle = bodyGrad;
  getCtx().beginPath();
  getCtx().arc(gx, gy, radius, 0, Math.PI * 2);
  getCtx().fill();

  getCtx().fillStyle = '#fff';
  getCtx().beginPath();
  getCtx().arc(gx - radius * 0.3, gy - radius * 0.2, radius * 0.2, 0, Math.PI * 2);
  getCtx().arc(gx + radius * 0.3, gy - radius * 0.2, radius * 0.2, 0, Math.PI * 2);
  getCtx().fill();
  getCtx().fillStyle = '#000';
  getCtx().beginPath();
  getCtx().arc(gx - radius * 0.3, gy - radius * 0.2, radius * 0.1, 0, Math.PI * 2);
  getCtx().arc(gx + radius * 0.3, gy - radius * 0.2, radius * 0.1, 0, Math.PI * 2);
  getCtx().fill();
}

// ─── Effects (ripples, explosions) ───────────────────────────────────

const RIPPLE_DURATION_S = 0.6;
const _rejectedRgb = 'rgba(233,69,96,';
const _optimisticRgb = 'rgba(0,180,216,';

function hexToRgb(hex: string): string {
  const h = hex.replace('#', '');
  const n = parseInt(h, 16);
  return `rgb(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255}`;
}

const _cachedPaletteRgb: string[] = PALETTE_COLORS.map(c => hexToRgb(c));

function rippleColor(
  ripple: { playerIndex: number; rejected?: boolean; isOptimistic?: boolean },
  playerMap: Map<number, { palette: number }>,
): { base: string; alpha: number } {
  if (ripple.rejected) return { base: _rejectedRgb, alpha: 1 };
  if (ripple.isOptimistic) return { base: _optimisticRgb, alpha: 1 };
  const player = playerMap.get(ripple.playerIndex);
  const idx = player
    ? player.palette % PALETTE_COLORS.length
    : ripple.playerIndex % PALETTE_COLORS.length;
  return { base: _cachedPaletteRgb[idx]!, alpha: 1 };
}

let _pruneScheduled = false;

export function pruneEffects(): void {
  if (_pruneScheduled) return;
  _pruneScheduled = true;
  requestAnimationFrame(() => {
    try {
      const currentRipples = getState().ripples;
      const remaining = currentRipples.filter(r => Date.now() - r.time <= RIPPLE_DURATION_S * 1000).slice(-50);
      if (remaining.length !== currentRipples.length) {
        dispatch({ type: 'SET_STATE', partial: { ripples: remaining } });
      }

      const explosion = getState().explosionEffect;
      if (explosion && Date.now() - explosion.startTime > 500) {
        dispatch({ type: 'SET_STATE', partial: { explosionEffect: null } });
      }
    } finally {
      _pruneScheduled = false;
    }
  });
}

export function drawRipples(now: number, playerMap: Map<number, { palette: number }>): void {
  const remaining = getState().ripples;
  const ctx = getCtx();
  for (const ripple of remaining) {
    const age = (now - ripple.time) / 1000;
    const t = Math.min(1, age / RIPPLE_DURATION_S);
    if (t >= 1) continue;

    const rx = ripple.x * $canvas.width;
    const ry = (1 - ripple.y) * $canvas.height;
    const maxRadius = Math.min($canvas.width, $canvas.height) * 0.06;
    const radius = maxRadius * (0.3 + 0.7 * t);
    const alpha = (1 - t) * 0.85;

    if (ripple.rejected) {
      ctx.globalAlpha = alpha;
      ctx.strokeStyle = _rejectedRgb + ')';
      ctx.lineWidth = 3;
      const s = 12 + 8 * t;
      ctx.beginPath();
      ctx.moveTo(rx - s, ry - s);
      ctx.lineTo(rx + s, ry + s);
      ctx.moveTo(rx + s, ry - s);
      ctx.lineTo(rx - s, ry + s);
      ctx.stroke();
    } else {
      const { base } = rippleColor(ripple, playerMap);
      ctx.beginPath();
      ctx.arc(rx, ry, radius, 0, Math.PI * 2);
      ctx.globalAlpha = alpha;
      ctx.strokeStyle = base + ')';
      ctx.lineWidth = 2;
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  }
}

export function drawExplosion(now: number): void {
  const explosion = getState().explosionEffect;
  if (!explosion) return;
  if (!gameImages['explosion']!.loaded) return;

  const elapsed = now - explosion.startTime;
  const duration = 500;
  const ctx = getCtx();
  const progress = elapsed / duration;
  const ex = explosion.x * $canvas.width;
  const ey = (1 - explosion.y) * $canvas.height;
  const baseSize = Math.min($canvas.width, $canvas.height) * 0.15;
  const size = baseSize * (0.5 + progress * 0.5);
  ctx.globalAlpha = 1 - progress;
  ctx.drawImage(gameImages['explosion']!.img, ex - size / 2, ey - size / 2, size, size);
  ctx.globalAlpha = 1;
}
