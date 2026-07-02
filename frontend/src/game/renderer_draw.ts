import { $canvas, getCtx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { state, getInterpolatedBalloon, getInterpolatedGhost, getInterpolatedBird } from './state.js';

export function drawBalloon(): void {
  const interp = getInterpolatedBalloon();
  const bx = interp.x * $canvas.width;
  const by = (1 - interp.y) * $canvas.height;
  const radius = Math.min($canvas.width, $canvas.height) * 0.06;

  if (gameImages['balloon']!.loaded) {
    const img: HTMLImageElement = gameImages['balloon']!.img;
    const w = radius * 2.5;
    const h = w * (img.height / img.width);
    const tilt = Math.max(-5, Math.min(5, state.wind * 40)) * (Math.PI / 180);
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

export function drawBird(): void {
  const bird = getInterpolatedBird();
  if (!bird || !bird.active) return;
  const bx: number = bird.x * $canvas.width;
  const by: number = (1 - bird.y) * $canvas.height;
  const size: number = Math.min($canvas.width, $canvas.height) * 0.035;

  // 飞行方向：根据水平速度决定朝向
  const vx = state.balloon.x - bird.x;
  const dir = vx >= 0 ? 1 : -1;

  // 翅膀扇动动画（基于时间）
  const flapPhase = Math.sin(Date.now() * 0.012);
  const wingY = flapPhase * size * 0.5;

  getCtx().save();
  getCtx().translate(bx, by);
  getCtx().scale(dir, 1);

  // ── 翅膀（后层）──
  getCtx().beginPath();
  getCtx().ellipse(-size * 0.15, -wingY * 0.6, size * 0.55, size * 0.28, -0.35, 0, Math.PI * 2);
  const wingGrad: CanvasGradient = getCtx().createLinearGradient(0, -size * 0.4, 0, size * 0.2);
  wingGrad.addColorStop(0, '#e85d04');
  wingGrad.addColorStop(1, '#dc2f02');
  getCtx().fillStyle = wingGrad;
  getCtx().fill();

  // ── 身体 ──
  getCtx().beginPath();
  getCtx().ellipse(0, 0, size * 0.6, size * 0.42, 0, 0, Math.PI * 2);
  const bodyGrad: CanvasGradient = getCtx().createRadialGradient(
    -size * 0.15, -size * 0.15, size * 0.05,
    0, 0, size * 0.6,
  );
  bodyGrad.addColorStop(0, '#ffba08');
  bodyGrad.addColorStop(0.6, '#f48c06');
  bodyGrad.addColorStop(1, '#e85d04');
  getCtx().fillStyle = bodyGrad;
  getCtx().fill();

  // ── 翅膀（前层，覆盖在身体上）──
  getCtx().beginPath();
  getCtx().ellipse(size * 0.05, wingY * 0.3, size * 0.45, size * 0.22, 0.3, 0, Math.PI * 2);
  getCtx().fillStyle = wingGrad;
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

export function drawGhost(): void {
  const interpGhost = getInterpolatedGhost();
  if (!interpGhost) return;
  const radius: number = Math.min($canvas.width, $canvas.height) * 0.035;
  const gx = interpGhost.x * $canvas.width;
  const gy = (1 - interpGhost.y) * $canvas.height;

  const isRepelled = state.ghost.repelTimer > 0;
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
      const flash: boolean = Math.sin(Date.now() * 0.02) > 0;
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
