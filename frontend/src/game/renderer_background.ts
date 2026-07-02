import { $canvas, getCtx } from './renderer_canvas.js';
import { getState } from './store.js';
import {
  bgState,
  cloudImages,
  CLOUD_Y_MAX,
  CLOUD_Y_MIN,
  ensureBackgroundInitialized,
  gameImages,
  randomCloudY,
  registerStaticCacheInvalidate,
  type Cloud,
} from './renderer_background_data.js';

export { initBackground, gameImages } from './renderer_background_data.js';

let staticCanvas: HTMLCanvasElement | null = null;
let staticCtx: CanvasRenderingContext2D | null = null;
let staticCacheW = 0;
let staticCacheH = 0;

export function invalidateBackgroundStaticCache(): void {
  staticCacheW = 0;
  staticCacheH = 0;
}

registerStaticCacheInvalidate(invalidateBackgroundStaticCache);

/** Half-width of a cloud in normalized canvas coordinates. */
function cloudHalfWidth(cloud: Cloud): number {
  return cloud.width * 0.55;
}

function respawnCloud(cloud: Cloud, side: 'left' | 'right'): void {
  const half = cloudHalfWidth(cloud);
  cloud.x = side === 'left' ? -half : 1 + half;
  cloud.y = randomCloudY();
  cloud.bobPhase = Math.random() * Math.PI * 2;
  cloud.variant = Math.floor(Math.random() * cloudImages.length);
}

function advanceCloud(cloud: Cloud, windDir: number): void {
  // Clouds follow wind direction; near-still when wind is calm.
  // windDir ranges ~-0.65..0.65; multiplier scales it to visible drift speed.
  cloud.x += cloud.speed * windDir * 20;
  const half = cloudHalfWidth(cloud);
  if (cloud.x - half > 1.02) {
    respawnCloud(cloud, 'left');
  } else if (cloud.x + half < -0.02) {
    respawnCloud(cloud, 'right');
  }
}

function drawProceduralCloud(target: CanvasRenderingContext2D, cx: number, cy: number, w: number, opacity: number): void {
  const h = w * 0.42;
  const alpha = Math.min(1, opacity);
  target.fillStyle = `rgba(90, 130, 170, ${alpha * 0.12})`;
  target.beginPath();
  target.ellipse(cx - w * 0.24 + 4, cy + h * 0.12 + 5, w * 0.3, h * 0.34, 0, 0, Math.PI * 2);
  target.ellipse(cx + 3, cy - h * 0.06 + 5, w * 0.34, h * 0.4, 0, 0, Math.PI * 2);
  target.ellipse(cx + w * 0.26 + 3, cy + h * 0.08 + 5, w * 0.28, h * 0.32, 0, 0, Math.PI * 2);
  target.fill();

  target.fillStyle = `rgba(255, 255, 255, ${alpha})`;
  target.beginPath();
  target.ellipse(cx - w * 0.24, cy + h * 0.12, w * 0.3, h * 0.34, 0, 0, Math.PI * 2);
  target.ellipse(cx, cy - h * 0.06, w * 0.34, h * 0.4, 0, 0, Math.PI * 2);
  target.ellipse(cx + w * 0.26, cy + h * 0.08, w * 0.28, h * 0.32, 0, 0, Math.PI * 2);
  target.ellipse(cx - w * 0.04, cy + h * 0.18, w * 0.26, h * 0.26, 0, 0, Math.PI * 2);
  target.fill();
}

function drawCloudSprite(target: CanvasRenderingContext2D, cloud: Cloud, cx: number, cy: number, cw: number): void {
  const imgEntry = cloudImages[cloud.variant % cloudImages.length];
  if (imgEntry?.loaded) {
    target.globalAlpha = Math.min(1, cloud.opacity);
    const imgW = cw * 2;
    const imgH = cw * 0.8;
    target.drawImage(imgEntry.img, cx - imgW / 2, cy - imgH / 2, imgW, imgH);
    target.globalAlpha = 1;
    return;
  }
  drawProceduralCloud(target, cx, cy, cw, cloud.opacity);
}

function drawMountainsTo(target: CanvasRenderingContext2D, w: number, h: number): void {
  if (gameImages['mountains']!.loaded) {
    const img = gameImages['mountains']!.img;
    const drawHeight = Math.min(
      w * (img.height / img.width),
      h * 0.4,
    );
    target.globalAlpha = 0.75;
    target.drawImage(img, 0, h - drawHeight, w, drawHeight);
    target.globalAlpha = 1;
    return;
  }

  target.fillStyle = 'rgba(30, 55, 90, 0.85)';
  target.beginPath();
  target.moveTo(0, h);
  for (const m of bgState.mountains) {
    const mx = m.x * w;
    const my = h - m.height * h;
    target.lineTo(mx, my);
    target.lineTo(mx + m.width * w * 0.5, h);
  }
  target.lineTo(w, h);
  target.closePath();
  target.fill();
}

function ensureStaticLayer(): void {
  const w = $canvas.width;
  const h = $canvas.height;
  if (staticCanvas && staticCtx && staticCacheW === w && staticCacheH === h) {
    return;
  }

  if (!staticCanvas) {
    staticCanvas = document.createElement('canvas');
  }
  staticCanvas.width = w;
  staticCanvas.height = h;
  staticCtx = staticCanvas.getContext('2d');
  if (!staticCtx) return;

  if (gameImages['sky']!.loaded) {
    staticCtx.drawImage(gameImages['sky']!.img, 0, 0, w, h);
  } else if (bgState.gradient) {
    staticCtx.fillStyle = bgState.gradient;
    staticCtx.fillRect(0, 0, w, h);
  } else {
    staticCtx.fillStyle = '#1a1a2e';
    staticCtx.fillRect(0, 0, w, h);
  }

  drawMountainsTo(staticCtx, w, h);
  staticCacheW = w;
  staticCacheH = h;
}

function drawStars(time: number): void {
  for (const star of bgState.stars) {
    if (star.y > 0.62) continue;
    const alpha = 0.55 + Math.sin(time * 1.4 + star.twinkle) * 0.35;
    getCtx().fillStyle = `rgba(255, 255, 255, ${alpha})`;
    getCtx().beginPath();
    getCtx().arc(star.x * $canvas.width, star.y * $canvas.height, star.size, 0, Math.PI * 2);
    getCtx().fill();
  }
}

function drawCloudLayer(time: number, windDir: number): void {
  for (const cloud of bgState.clouds) {
    advanceCloud(cloud, windDir);

    const cx = cloud.x * $canvas.width;
    const bob = Math.sin(time * 0.35 + cloud.bobPhase) * 0.012;
    const yNorm = Math.min(CLOUD_Y_MAX, Math.max(CLOUD_Y_MIN, cloud.y + bob));
    const cy = yNorm * $canvas.height;
    const cw = cloud.width * $canvas.width;

    drawCloudSprite(getCtx(), cloud, cx, cy, cw);
  }
}

function drawParticles(windDir: number): void {
  for (const p of bgState.particles) {
    p.x += windDir * 0.0008;
    p.y += 0.0001;
    p.life -= 0.005;
    if (p.life <= 0 || p.x < -0.05 || p.x > 1.05) {
      p.x = Math.random();
      p.y = Math.random() * 0.8;
      p.life = 1;
    }
    const alpha = p.life * 0.3;
    getCtx().fillStyle = `rgba(200, 220, 255, ${alpha})`;
    getCtx().beginPath();
    getCtx().arc(p.x * $canvas.width, p.y * $canvas.height, p.size, 0, Math.PI * 2);
    getCtx().fill();
  }
}

export function drawBackground(now: number): void {
  ensureBackgroundInitialized();
  ensureStaticLayer();

  if (staticCanvas) {
    getCtx().drawImage(staticCanvas, 0, 0);
  }

  const time = now * 0.001;
  const windDir = getState().wind || 0;

  drawStars(time);
  drawCloudLayer(time, windDir);
  drawParticles(windDir);
}
