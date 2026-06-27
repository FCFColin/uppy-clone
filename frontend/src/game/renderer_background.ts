import { $canvas, ctx } from './renderer_canvas.js';
import { state } from './state.js';
import { bgState, gameImages, initBackground } from './renderer_background_data.js';

export { initBackground, gameImages } from './renderer_background_data.js';

export function drawBackground(): void {
  if (!bgState.gradient) initBackground();

  if (gameImages['sky']!.loaded) {
    ctx.drawImage(gameImages['sky']!.img, 0, 0, $canvas.width, $canvas.height);
  } else if (bgState.gradient) {
    ctx.fillStyle = bgState.gradient;
    ctx.fillRect(0, 0, $canvas.width, $canvas.height);
  }

  const time: number = Date.now() * 0.001;
  for (const star of bgState.stars) {
    const alpha: number = 0.45 + Math.sin(time + star.twinkle) * 0.12;
    ctx.fillStyle = `rgba(255, 255, 255, ${alpha})`;
    ctx.beginPath();
    ctx.arc(star.x * $canvas.width, star.y * $canvas.height, star.size, 0, Math.PI * 2);
    ctx.fill();
  }

  if (gameImages['mountains']!.loaded) {
    const img: HTMLImageElement = gameImages['mountains']!.img;
    const drawHeight: number = Math.min(
      $canvas.width * (img.height / img.width),
      $canvas.height * 0.4
    );
    ctx.globalAlpha = 0.75;
    ctx.drawImage(img, 0, $canvas.height - drawHeight, $canvas.width, drawHeight);
    ctx.globalAlpha = 1;
  } else {
    ctx.fillStyle = 'rgba(15, 23, 41, 0.6)';
    ctx.beginPath();
    ctx.moveTo(0, $canvas.height);
    for (const m of bgState.mountains) {
      const mx: number = m.x * $canvas.width;
      const my: number = $canvas.height - m.height * $canvas.height;
      ctx.lineTo(mx, my);
      ctx.lineTo(mx + m.width * $canvas.width * 0.5, $canvas.height);
    }
    ctx.lineTo($canvas.width, $canvas.height);
    ctx.closePath();
    ctx.fill();
  }

  for (const cloud of bgState.clouds) {
    cloud.x += cloud.speed;
    if (cloud.x > 1.3) cloud.x = -0.3;
    const cx: number = cloud.x * $canvas.width;
    const cy: number = cloud.y * $canvas.height;
    const cw: number = cloud.width * $canvas.width;

    if (gameImages['cloud']!.loaded) {
      ctx.globalAlpha = Math.min(1, cloud.opacity * 5);
      const imgW: number = cw * 2;
      const imgH: number = cw * 0.8;
      ctx.drawImage(gameImages['cloud']!.img, cx - imgW / 2, cy - imgH / 2, imgW, imgH);
      ctx.globalAlpha = 1;
    } else {
      const r: number = cloud.layer === 0 ? 80 : (cloud.layer === 1 ? 120 : 160);
      const g: number = cloud.layer === 0 ? 120 : (cloud.layer === 1 ? 150 : 180);
      const b: number = cloud.layer === 0 ? 180 : (cloud.layer === 1 ? 200 : 220);
      ctx.fillStyle = `rgba(${r}, ${g}, ${b}, ${cloud.opacity})`;
      ctx.beginPath();
      ctx.ellipse(cx, cy, cw, cw * 0.4, 0, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  const windDir: number = state.wind || 0;
  for (const p of bgState.particles) {
    p.x += windDir * 0.0008;
    p.y += 0.0001;
    p.life -= 0.005;
    if (p.life <= 0 || p.x < -0.05 || p.x > 1.05) {
      p.x = Math.random();
      p.y = Math.random() * 0.8;
      p.life = 1;
    }
    const alpha: number = p.life * 0.3;
    ctx.fillStyle = `rgba(200, 220, 255, ${alpha})`;
    ctx.beginPath();
    ctx.arc(p.x * $canvas.width, p.y * $canvas.height, p.size, 0, Math.PI * 2);
    ctx.fill();
  }
}
