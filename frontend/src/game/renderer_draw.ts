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
  const size: number = Math.min($canvas.width, $canvas.height) * 0.03;

  ctx.beginPath();
  ctx.moveTo(bx + size, by);
  ctx.lineTo(bx - size, by - size * 0.7);
  ctx.lineTo(bx - size, by + size * 0.7);
  ctx.closePath();
  ctx.fillStyle = '#fca311';
  ctx.fill();

  ctx.beginPath();
  ctx.arc(bx + size * 0.3, by - size * 0.1, size * 0.15, 0, Math.PI * 2);
  ctx.fillStyle = '#000';
  ctx.fill();
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
