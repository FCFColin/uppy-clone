import {
  LAKE_RATIO,
  cloudImages,
  ensureStaticLayer,
  getStaticCanvas,
  invalidateStaticCache,
  resetStaticBackground,
} from './background_static.js';
import { getDevicePixelRatio } from './renderer.js';

function cssCanvasSize(canvas: HTMLCanvasElement): { w: number; h: number; dpr: number } {
  const dpr = getDevicePixelRatio() || 1;
  const w = canvas.clientWidth || Math.max(1, Math.floor(canvas.width / dpr));
  const h = canvas.clientHeight || Math.max(1, Math.floor(canvas.height / dpr));
  return { w, h, dpr };
}

interface Star {
  x: number;
  y: number;
  size: number;
  twinkle: number;
  baseAlpha: number;
}

interface Cloud {
  x: number;
  y: number;
  width: number;
  speed: number;
  opacity: number;
  layer: number;
  variant: number;
  bobPhase: number;
}

interface CloudLayerConfig {
  count: number;
  yMin: number;
  yRange: number;
  widthMin: number;
  widthRange: number;
  speedMin: number;
  speedRange: number;
  opacityMin: number;
  opacityRange: number;
  layer: number;
}

const CLOUD_Y_MIN = 0.06;
const CLOUD_Y_MAX = 0.52;

const CLOUD_LAYERS: CloudLayerConfig[] = [
  { count: 4, yMin: 0.1, yRange: 0.2, widthMin: 0.14, widthRange: 0.1, speedMin: 0.00004, speedRange: 0.00003, opacityMin: 0.35, opacityRange: 0.2, layer: 0 },
  { count: 3, yMin: 0.25, yRange: 0.15, widthMin: 0.11, widthRange: 0.08, speedMin: 0.00007, speedRange: 0.00004, opacityMin: 0.45, opacityRange: 0.15, layer: 1 },
  { count: 3, yMin: 0.38, yRange: 0.12, widthMin: 0.08, widthRange: 0.06, speedMin: 0.0001, speedRange: 0.00005, opacityMin: 0.55, opacityRange: 0.15, layer: 2 },
];

const dynamicState = {
  stars: [] as Star[],
  clouds: [] as Cloud[],
};

let _frameCounter = 0;

let _prefersReducedMotion: boolean | null = null;

function prefersReducedMotion(): boolean {
  if (_prefersReducedMotion === null) {
    _prefersReducedMotion =
      typeof window !== 'undefined' && typeof window.matchMedia === 'function'
        ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
        : false;
  }
  return _prefersReducedMotion;
}

function randomCloudY(): number {
  return CLOUD_Y_MIN + Math.random() * (CLOUD_Y_MAX - CLOUD_Y_MIN);
}

function spawnCloud(layerCfg: CloudLayerConfig, x?: number): Cloud {
  return {
    x: x ?? Math.random(),
    y: layerCfg.yMin + Math.random() * layerCfg.yRange,
    width: layerCfg.widthMin + Math.random() * layerCfg.widthRange,
    speed: layerCfg.speedMin + Math.random() * layerCfg.speedRange,
    opacity: layerCfg.opacityMin + Math.random() * layerCfg.opacityRange,
    layer: layerCfg.layer,
    variant: Math.floor(Math.random() * cloudImages.length),
    bobPhase: Math.random() * Math.PI * 2,
  };
}

function initDynamicBackground(): void {
  dynamicState.stars = Array.from({ length: 90 }, () => {
    const x = Math.random();
    const y = Math.random() * 0.7;
    const size = 0.3 + Math.random() * 0.9;
    const baseAlpha = 0.3 + Math.random() * 0.4;
    return {
      x,
      y,
      size,
      twinkle: Math.random() * Math.PI * 2,
      baseAlpha,
    };
  });

  dynamicState.clouds = CLOUD_LAYERS.flatMap((cfg) => Array.from({ length: cfg.count }, () => spawnCloud(cfg)));
}

function cloudHalfWidth(cloud: Cloud): number {
  return cloud.width * 0.6;
}

function respawnCloud(cloud: Cloud, side: 'left' | 'right'): void {
  const half = cloudHalfWidth(cloud);
  cloud.x = side === 'left' ? -half : 1 + half;
  cloud.y = randomCloudY();
  cloud.bobPhase = Math.random() * Math.PI * 2;
  cloud.variant = Math.floor(Math.random() * cloudImages.length);
}

function advanceCloud(cloud: Cloud, windDir: number): void {
  cloud.x += cloud.speed * windDir * 20;
  const half = cloudHalfWidth(cloud);
  if (cloud.x - half > 1.02) {
    respawnCloud(cloud, 'left');
  } else if (cloud.x + half < -0.02) {
    respawnCloud(cloud, 'right');
  }
}

function drawProceduralCloud(ctx: CanvasRenderingContext2D, cx: number, cy: number, w: number, opacity: number): void {
  const h = w * 0.42;
  const alpha = Math.min(1, opacity);
  const puffs: [number, number, number, number][] = [
    [-0.30, 0.05, 0.22, 0.28],
    [-0.10, -0.08, 0.28, 0.35],
    [0.10, -0.06, 0.30, 0.36],
    [0.28, 0.04, 0.22, 0.28],
    [0.0, 0.12, 0.20, 0.20],
  ];

  ctx.fillStyle = `rgba(220, 225, 240, ${alpha * 0.55})`;
  ctx.beginPath();
  for (const [dx, dy, rx, ry] of puffs) {
    ctx.ellipse(cx + dx * w, cy + dy * h + h * 0.08, w * rx, h * ry, 0, 0, Math.PI * 2);
  }
  ctx.fill();

  ctx.fillStyle = `rgba(240, 242, 250, ${alpha * 0.65})`;
  ctx.beginPath();
  for (const [dx, dy, rx, ry] of puffs) {
    ctx.ellipse(cx + dx * w, cy + dy * h - h * 0.05, w * rx * 0.9, h * ry * 0.85, 0, 0, Math.PI * 2);
  }
  ctx.fill();
}

function drawCloudSprite(ctx: CanvasRenderingContext2D, cloud: Cloud, cx: number, cy: number, cw: number): void {
  const imgEntry = cloudImages[cloud.variant % cloudImages.length];
  if (imgEntry?.loaded) {
    ctx.globalAlpha = Math.min(1, cloud.opacity);
    const imgW = cw * 2;
    const imgH = cw * 0.85;
    ctx.drawImage(imgEntry.img, cx - imgW / 2, cy - imgH / 2, imgW, imgH);
    ctx.globalAlpha = 1;
    return;
  }
  drawProceduralCloud(ctx, cx, cy, cw, cloud.opacity);
}

function drawStars(ctx: CanvasRenderingContext2D, canvas: HTMLCanvasElement, time: number): void {
  const { w, h } = cssCanvasSize(canvas);
  const reduced = prefersReducedMotion();
  for (const star of dynamicState.stars) {
    if (star.y > 0.75) continue;
    const twinkle = reduced ? 0 : Math.sin(time * 1.3 + star.twinkle);
    const alpha = star.baseAlpha * (reduced ? 0.9 : 1 + twinkle * 0.15);
    const a = Math.max(0.05, alpha);
    ctx.globalAlpha = a;
    ctx.fillStyle = 'rgba(220, 225, 255, 1)';
    ctx.beginPath();
    ctx.arc(star.x * w, star.y * h, star.size, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;
}

function drawClouds(ctx: CanvasRenderingContext2D, canvas: HTMLCanvasElement, time: number, windDir: number): void {
  const { w, h } = cssCanvasSize(canvas);
  const reduced = prefersReducedMotion();
  const shouldAdvance = !reduced && _frameCounter % 3 === 0;
  for (const cloud of dynamicState.clouds) {
    if (shouldAdvance) advanceCloud(cloud, windDir);
    const cx = cloud.x * w;
    const bob = reduced ? 0 : Math.sin(time * 0.35 + cloud.bobPhase) * 0.012;
    const yNorm = Math.min(CLOUD_Y_MAX, Math.max(CLOUD_Y_MIN, cloud.y + bob));
    drawCloudSprite(ctx, cloud, cx, yNorm * h, cloud.width * w);
  }
}

function drawLakeReflection(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  time: number,
  dpr: number,
): void {
  const staticCanvas = getStaticCanvas();
  if (!staticCanvas) return;
  const { w, h } = cssCanvasSize(canvas);
  const horizon = h * (1 - LAKE_RATIO);
  const lakeH = h - horizon;
  const reduced = prefersReducedMotion();
  const bands = 5;
  const bandH = lakeH / bands;

  for (let i = 0; i < bands; i++) {
    const y = horizon + i * bandH;
    const dist = i / (bands - 1);
    const baseAlpha = 0.35 * (1 - dist * 0.6);
    const offset = reduced
      ? 0
      : Math.sin(time * 1.6 + i * 0.5) * (2 + dist * 4);

    const srcY = (horizon - (i + 1) * (horizon / bands)) * dpr;
    const srcH = (horizon / bands) * dpr;
    const drawH = bandH + 1;

    ctx.save();
    ctx.globalAlpha = baseAlpha;
    ctx.drawImage(
      staticCanvas,
      0,
      Math.max(0, srcY),
      w * dpr,
      Math.max(1, srcH),
      offset,
      y,
      w,
      drawH,
    );
    ctx.restore();
  }

  ctx.save();
  ctx.globalCompositeOperation = 'source-over';
  ctx.fillStyle = 'rgba(5, 3, 18, 0.3)';
  ctx.fillRect(0, horizon + lakeH * 0.6, w, lakeH * 0.4);
  ctx.restore();
}

export function invalidateBackgroundCache(): void {
  invalidateStaticCache();
}

export function resetBackgroundState(): void {
  dynamicState.stars = [];
  dynamicState.clouds = [];
  _frameCounter = 0;
  resetStaticBackground();
}

export function drawBackground(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  now: number,
  windDir: number,
): void {
  const { w, h, dpr } = cssCanvasSize(canvas);
  ensureStaticLayer(canvas, w, h, dpr);
  if (dynamicState.stars.length === 0) initDynamicBackground();
  const staticCanvas = getStaticCanvas();
  if (staticCanvas) ctx.drawImage(staticCanvas, 0, 0, w, h);
  const time = now * 0.001;
  drawStars(ctx, canvas, time);
  drawClouds(ctx, canvas, time, windDir);
  drawLakeReflection(ctx, canvas, time, dpr);
  _frameCounter++;
}
