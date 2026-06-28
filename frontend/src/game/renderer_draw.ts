import { $canvas, ctx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { state, getInterpolatedBalloon, getInterpolatedGhost, getInterpolatedBird } from './state.js';

export function drawBalloon(): void {
  const interp = getInterpolatedBalloon();
  const bx: number = interp.x * $canvas.width;
  const by: number = (1 - interp.y) * $canvas.height;
  const radius: number = Math.min($canvas.width, $canvas.height) * 0.06;

  if (gameImages['balloon']!.loaded) {
    const img: HTMLImageElement = gameImages['balloon']!.img;
    const w: number = radius * 2.5;
    const h: number = w * (img.height / img.width);
    const tilt = Math.max(-5, Math.min(5, state.wind * 40)) * (Math.PI / 180);
    ctx.save();
    ctx.translate(bx, by);
    ctx.rotate(tilt);
    ctx.drawImage(img, -w / 2, -h / 2, w, h);
    ctx.restore();
    return;
  }

  ctx.beginPath();
  ctx.arc(bx, by, radius, 0, Math.PI * 2);
  const balloonGrad: CanvasGradient = ctx.createRadialGradient(
    bx - radius * 0.3, by - radius * 0.3, radius * 0.1,
    bx, by, radius
  );
  balloonGrad.addColorStop(0, '#ff6b6b');
  balloonGrad.addColorStop(1, '#e94560');
  ctx.fillStyle = balloonGrad;
  ctx.fill();

  ctx.beginPath();
  ctx.arc(bx - radius * 0.25, by - radius * 0.25, radius * 0.2, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(255,255,255,0.3)';
  ctx.fill();

  ctx.beginPath();
  ctx.moveTo(bx, by + radius);
  ctx.lineTo(bx, by + radius + radius * 0.8);
  ctx.strokeStyle = '#aaa';
  ctx.lineWidth = 2;
  ctx.stroke();
}

export function drawBird(): void {
  const bird = getInterpolatedBird();
  if (!bird || !bird.active) return;
  const bx: number = bird.x * $canvas.width;
  const by: number = (1 - bird.y) * $canvas.height;
  const size: number = Math.min($canvas.width, $canvas.height) * 0.035;

  // 飞行方向：根据水平速度决定朝向
  const vx = state.balloon.x - bird.x;
  const dir: number = vx >= 0 ? 1 : -1;

  // 翅膀扇动动画（基于时间）
  const flapPhase: number = Math.sin(Date.now() * 0.012);
  const wingY: number = flapPhase * size * 0.5;

  ctx.save();
  ctx.translate(bx, by);
  ctx.scale(dir, 1);

  // ── 翅膀（后层）──
  ctx.beginPath();
  ctx.ellipse(-size * 0.15, -wingY * 0.6, size * 0.55, size * 0.28, -0.35, 0, Math.PI * 2);
  const wingGrad: CanvasGradient = ctx.createLinearGradient(0, -size * 0.4, 0, size * 0.2);
  wingGrad.addColorStop(0, '#e85d04');
  wingGrad.addColorStop(1, '#dc2f02');
  ctx.fillStyle = wingGrad;
  ctx.fill();

  // ── 身体 ──
  ctx.beginPath();
  ctx.ellipse(0, 0, size * 0.6, size * 0.42, 0, 0, Math.PI * 2);
  const bodyGrad: CanvasGradient = ctx.createRadialGradient(
    -size * 0.15, -size * 0.15, size * 0.05,
    0, 0, size * 0.6,
  );
  bodyGrad.addColorStop(0, '#ffba08');
  bodyGrad.addColorStop(0.6, '#f48c06');
  bodyGrad.addColorStop(1, '#e85d04');
  ctx.fillStyle = bodyGrad;
  ctx.fill();

  // ── 翅膀（前层，覆盖在身体上）──
  ctx.beginPath();
  ctx.ellipse(size * 0.05, wingY * 0.3, size * 0.45, size * 0.22, 0.3, 0, Math.PI * 2);
  ctx.fillStyle = wingGrad;
  ctx.fill();

  // ── 尾巴 ──
  ctx.beginPath();
  ctx.moveTo(-size * 0.5, 0);
  ctx.lineTo(-size * 0.85, -size * 0.2);
  ctx.lineTo(-size * 0.8, 0);
  ctx.lineTo(-size * 0.85, size * 0.2);
  ctx.closePath();
  ctx.fillStyle = '#dc2f02';
  ctx.fill();

  // ── 喙 ──
  ctx.beginPath();
  ctx.moveTo(size * 0.55, -size * 0.05);
  ctx.lineTo(size * 0.8, 0);
  ctx.lineTo(size * 0.55, size * 0.1);
  ctx.closePath();
  ctx.fillStyle = '#ffba08';
  ctx.fill();

  // ── 眼睛 ──
  ctx.beginPath();
  ctx.arc(size * 0.32, -size * 0.12, size * 0.1, 0, Math.PI * 2);
  ctx.fillStyle = '#fff';
  ctx.fill();
  ctx.beginPath();
  ctx.arc(size * 0.35, -size * 0.12, size * 0.05, 0, Math.PI * 2);
  ctx.fillStyle = '#000';
  ctx.fill();

  ctx.restore();
}

export function drawGhost(): void {
  const interpGhost = getInterpolatedGhost();
  if (!interpGhost) return;
  const radius: number = Math.min($canvas.width, $canvas.height) * 0.035;
  const gx: number = interpGhost.x * $canvas.width;
  const gy: number = (1 - interpGhost.y) * $canvas.height;

  const isRepelled: boolean = state.ghost.repelTimer > 0;
  const baseColor: string = isRepelled ? '255, 100, 100' : '180, 100, 255';

  if (gameImages['ghost']!.loaded) {
    const size: number = radius * 4;
    if (isRepelled) {
      const glowGrad: CanvasGradient = ctx.createRadialGradient(gx, gy, 0, gx, gy, size * 0.7);
      glowGrad.addColorStop(0, 'rgba(255, 50, 50, 0.6)');
      glowGrad.addColorStop(1, 'rgba(255, 50, 50, 0)');
      ctx.fillStyle = glowGrad;
      ctx.beginPath();
      ctx.arc(gx, gy, size * 0.7, 0, Math.PI * 2);
      ctx.fill();
      const flash: boolean = Math.sin(Date.now() * 0.02) > 0;
      ctx.globalAlpha = flash ? 0.6 : 1;
    }
    ctx.drawImage(gameImages['ghost']!.img, gx - size / 2, gy - size / 2, size, size);
    ctx.globalAlpha = 1;
    return;
  }

  const glowGrad: CanvasGradient = ctx.createRadialGradient(gx, gy, 0, gx, gy, radius * 2);
  glowGrad.addColorStop(0, `rgba(${baseColor}, 0.4)`);
  glowGrad.addColorStop(1, `rgba(${baseColor}, 0)`);
  ctx.fillStyle = glowGrad;
  ctx.beginPath();
  ctx.arc(gx, gy, radius * 2, 0, Math.PI * 2);
  ctx.fill();

  const bodyGrad: CanvasGradient = ctx.createRadialGradient(gx - radius * 0.3, gy - radius * 0.3, 0, gx, gy, radius);
  bodyGrad.addColorStop(0, `rgba(${baseColor}, 0.9)`);
  bodyGrad.addColorStop(1, `rgba(${baseColor}, 0.5)`);
  ctx.fillStyle = bodyGrad;
  ctx.beginPath();
  ctx.arc(gx, gy, radius, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = '#fff';
  ctx.beginPath();
  ctx.arc(gx - radius * 0.3, gy - radius * 0.2, radius * 0.2, 0, Math.PI * 2);
  ctx.arc(gx + radius * 0.3, gy - radius * 0.2, radius * 0.2, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = '#000';
  ctx.beginPath();
  ctx.arc(gx - radius * 0.3, gy - radius * 0.2, radius * 0.1, 0, Math.PI * 2);
  ctx.arc(gx + radius * 0.3, gy - radius * 0.2, radius * 0.1, 0, Math.PI * 2);
  ctx.fill();
}

export { drawRipples, drawExplosion } from './renderer_draw_effects.js';
