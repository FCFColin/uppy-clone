import { $canvas, getCtx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { getState } from './store.js';
import { getInterpolatedBalloon, getInterpolatedGhost, getInterpolatedBird } from './state_interp.js';

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

  if (gameImages['balloon']!.loaded) {
    const img: HTMLImageElement = gameImages['balloon']!.img;
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

export function drawBird(now: number): void {
  const bird = getInterpolatedBird();
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

  // ── 尾巴 ──
  getCtx().beginPath();
  getCtx().moveTo(-size * 0.5, 0);
  getCtx().lineTo(-size * 0.85, -size * 0.2);
  getCtx().lineTo(-size * 0.8, 0);
  getCtx().lineTo(-size * 0.85, size * 0.2);
  getCtx().closePath();
  getCtx().fillStyle = '#dc2f02';
  getCtx().fill();

  // ── 喙 ──
  getCtx().beginPath();
  getCtx().moveTo(size * 0.55, -size * 0.05);
  getCtx().lineTo(size * 0.8, 0);
  getCtx().lineTo(size * 0.55, size * 0.1);
  getCtx().closePath();
  getCtx().fillStyle = '#ffba08';
  getCtx().fill();

  // ── 眼睛 ──
  getCtx().beginPath();
  getCtx().arc(size * 0.32, -size * 0.12, size * 0.1, 0, Math.PI * 2);
  getCtx().fillStyle = '#fff';
  getCtx().fill();
  getCtx().beginPath();
  getCtx().arc(size * 0.35, -size * 0.12, size * 0.05, 0, Math.PI * 2);
  getCtx().fillStyle = '#000';
  getCtx().fill();

  getCtx().restore();
}

export function drawGhost(now: number): void {
  const interpGhost = getInterpolatedGhost();
  if (!interpGhost) return;
  const radius: number = Math.min($canvas.width, $canvas.height) * 0.035;
  const gx = interpGhost.x * $canvas.width;
  const gy = (1 - interpGhost.y) * $canvas.height;

  const isRepelled = getState().ghost.repelTimer > 0;
  const baseColor = isRepelled ? '255, 100, 100' : '180, 100, 255';

  if (gameImages['ghost']!.loaded) {
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
    getCtx().drawImage(gameImages['ghost']!.img, gx - size / 2, gy - size / 2, size, size);
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

export { drawRipples, drawExplosion } from './renderer_draw_effects.js';
