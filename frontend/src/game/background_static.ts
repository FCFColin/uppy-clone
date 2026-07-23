export interface ImageEntry {
  img: HTMLImageElement;
  loaded: boolean;
  url: string;
  fallback: string;
}

export interface MountainPeak {
  x: number;
  y: number;
  cx: number;
  cy: number;
}

export interface MountainLayer {
  color: string;
  peaks: MountainPeak[];
}

interface StaticState {
  gradient: CanvasGradient | null;
  mountains: MountainLayer[];
  trees: number[];
}

export const LAKE_RATIO = 0.12;

export const bgImages: Record<string, ImageEntry> = {
  sky: { img: new Image(), loaded: false, url: '', fallback: '/assets/fallback/sky-bg.svg' },
  mountains: { img: new Image(), loaded: false, url: '', fallback: '/assets/fallback/mountains.svg' },
};

export const cloudImages: ImageEntry[] = [
  { img: new Image(), loaded: false, url: '', fallback: '/assets/fallback/cloud-1.svg' },
  { img: new Image(), loaded: false, url: '', fallback: '/assets/fallback/cloud-2.svg' },
  { img: new Image(), loaded: false, url: '', fallback: '/assets/fallback/cloud-3.svg' },
  { img: new Image(), loaded: false, url: '', fallback: '/assets/fallback/cloud-1.svg' },
];

const staticState: StaticState = {
  gradient: null,
  mountains: [],
  trees: [],
};

let staticCanvas: HTMLCanvasElement | null = null;
let staticCtx: CanvasRenderingContext2D | null = null;
let staticCacheW = 0;
let staticCacheH = 0;
let imagesInitialized = false;

function loadImageEntry(entry: ImageEntry, cacheKey: string): void {
  if (!entry.url) return;
  entry.img.onload = () => {
    entry.loaded = true;
    if (cacheKey === 'sky' || cacheKey === 'mountains') invalidateStaticCache();
  };
  entry.img.onerror = () => {
    entry.img.onerror = null;
    entry.img.src = entry.fallback;
  };
  entry.img.src = entry.url;
}

function initImages(): void {
  if (imagesInitialized) return;
  imagesInitialized = true;
  for (const key in bgImages) loadImageEntry(bgImages[key]!, key);
  for (const entry of cloudImages) loadImageEntry(entry, entry.url);
}

function generateMountains(w: number, horizon: number): MountainLayer[] {
  const palette = [
    { color: 'rgba(60, 55, 100, 0.3)' },
    { color: 'rgba(35, 30, 70, 0.6)' },
    { color: 'rgba(15, 12, 40, 0.9)' },
  ];
  return palette.map((p, li) => {
    const count = 6 + li * 2;
    const step = 1.45 / count;
    const baseHeight = horizon * (0.08 + li * 0.075);
    const peaks: MountainPeak[] = [];
    let x = -0.2;
    for (let i = 0; i <= count; i++) {
      const nextX = -0.2 + i * step;
      const heightVar = 0.45 + Math.random() * 0.85 + Math.sin(i * 0.9 + li) * 0.18;
      const h = baseHeight * Math.max(0.35, heightVar);
      peaks.push({
        x: nextX,
        y: horizon,
        cx: x + (nextX - x) * 0.5 + (Math.random() - 0.5) * 0.05,
        cy: horizon - h,
      });
      x = nextX;
    }
    return { color: p.color, peaks };
  });
}

function createSkyGradient(ctx: CanvasRenderingContext2D, cssH: number): CanvasGradient {
  const h = cssH;
  const g = ctx.createLinearGradient(0, 0, 0, h);
  g.addColorStop(0, '#0c1333');
  g.addColorStop(0.35, '#2a2060');
  g.addColorStop(0.7, '#7a5090');
  g.addColorStop(0.92, '#c090b0');
  g.addColorStop(1, '#d8a8a0');
  return g;
}

function drawSkyTo(ctx: CanvasRenderingContext2D, w: number, h: number): void {
  if (staticState.gradient) {
    ctx.fillStyle = staticState.gradient;
    ctx.fillRect(0, 0, w, h);
  } else {
    ctx.fillStyle = '#0c1333';
    ctx.fillRect(0, 0, w, h);
  }

  const horizon = h * (1 - LAKE_RATIO);
  const glow = ctx.createLinearGradient(0, horizon - h * 0.2, 0, horizon);
  glow.addColorStop(0, 'rgba(200, 160, 180, 0)');
  glow.addColorStop(1, 'rgba(200, 160, 180, 0.15)');
  ctx.fillStyle = glow;
  ctx.fillRect(0, horizon - h * 0.2, w, h * 0.2);
}

function drawMoonTo(ctx: CanvasRenderingContext2D, w: number, h: number): void {
  const mx = w * 0.8;
  const my = h * 0.15;
  const r = Math.min(w, h) * 0.075;

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  const outerGlow = ctx.createRadialGradient(mx, my, r * 2.5, mx, my, r * 5);
  outerGlow.addColorStop(0, 'rgba(200, 200, 230, 0.12)');
  outerGlow.addColorStop(1, 'rgba(200, 200, 230, 0)');
  ctx.fillStyle = outerGlow;
  ctx.beginPath();
  ctx.arc(mx, my, r * 5, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  const body = ctx.createRadialGradient(mx - r * 0.3, my - r * 0.3, r * 0.1, mx, my, r);
  body.addColorStop(0, '#f5f3f0');
  body.addColorStop(0.5, '#d0ccd8');
  body.addColorStop(1, '#a8a4b8');
  ctx.fillStyle = body;
  ctx.beginPath();
  ctx.arc(mx, my, r, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = 'rgba(255, 255, 255, 0.4)';
  ctx.beginPath();
  ctx.ellipse(mx - r * 0.35, my - r * 0.35, r * 0.18, r * 0.12, -0.4, 0, Math.PI * 2);
  ctx.fill();
}

function drawMountainsTo(ctx: CanvasRenderingContext2D, w: number, h: number): void {
  const horizon = h * (1 - LAKE_RATIO);
  for (const layer of staticState.mountains) {
    ctx.beginPath();
    ctx.moveTo(-0.25 * w, horizon);
    for (const p of layer.peaks) ctx.quadraticCurveTo(p.cx * w, p.cy, p.x * w, p.y);
    ctx.lineTo(1.25 * w, horizon);
    ctx.closePath();
    ctx.fillStyle = layer.color;
    ctx.fill();
  }

  const nearLayer = staticState.mountains[2];
  if (nearLayer) {
    const threshold = horizon * 0.18;
    ctx.fillStyle = 'rgba(220, 220, 235, 0.6)';
    for (let i = 0; i < nearLayer.peaks.length; i++) {
      const p = nearLayer.peaks[i]!;
      const peakHeight = horizon - p.cy;
      if (peakHeight < threshold) continue;
      const prev = nearLayer.peaks[i - 1] ?? { cx: p.cx - 0.06, cy: horizon };
      const next = nearLayer.peaks[i + 1] ?? { cx: p.cx + 0.06, cy: horizon };
      const snowBaseY = p.cy + peakHeight * 0.28;
      ctx.beginPath();
      ctx.moveTo(((p.cx + prev.cx) * 0.5) * w, snowBaseY);
      ctx.lineTo(p.cx * w, p.cy);
      ctx.lineTo(((p.cx + next.cx) * 0.5) * w, snowBaseY);
      ctx.closePath();
      ctx.fill();
    }
  }

  const haze = ctx.createLinearGradient(0, horizon - h * 0.1, 0, horizon);
  haze.addColorStop(0, 'rgba(40, 35, 80, 0)');
  haze.addColorStop(1, 'rgba(20, 15, 50, 0.25)');
  ctx.fillStyle = haze;
  ctx.fillRect(0, horizon - h * 0.1, w, h * 0.1);
}

function drawPineTree(ctx: CanvasRenderingContext2D, x: number, baseY: number, treeH: number): void {
  const layers = 3;
  const trunkW = treeH * 0.08;
  const trunkH = treeH * 0.2;

  ctx.fillStyle = 'rgba(8, 5, 25, 0.9)';
  ctx.fillRect(x - trunkW / 2, baseY - trunkH, trunkW, trunkH);

  for (let i = 0; i < layers; i++) {
    const layerY = baseY - trunkH - i * (treeH * 0.22);
    const layerW = treeH * (0.38 - i * 0.08);
    const layerH = treeH * 0.28;
    ctx.beginPath();
    ctx.moveTo(x - layerW, layerY);
    ctx.lineTo(x, layerY - layerH);
    ctx.lineTo(x + layerW, layerY);
    ctx.closePath();
    ctx.fill();
  }
}

function drawForestTo(ctx: CanvasRenderingContext2D, w: number, h: number): void {
  const horizon = h * (1 - LAKE_RATIO);
  for (const t of staticState.trees) {
    const x = t * w;
    const distFromCenter = Math.abs(t - 0.5) * 2;
    const edgeFactor = 0.6 + distFromCenter * 0.4;
    const treeH = h * (0.03 + Math.random() * 0.02) * edgeFactor;
    drawPineTree(ctx, x, horizon, treeH);
  }
}

function drawLakeBaseTo(ctx: CanvasRenderingContext2D, w: number, h: number): void {
  const horizon = h * (1 - LAKE_RATIO);
  const g = ctx.createLinearGradient(0, horizon, 0, h);
  g.addColorStop(0, 'rgba(15, 10, 40, 0.92)');
  g.addColorStop(1, 'rgba(5, 3, 18, 1)');
  ctx.fillStyle = g;
  ctx.fillRect(0, horizon, w, h - horizon);
}

export function invalidateStaticCache(): void {
  staticCacheW = 0;
  staticCacheH = 0;
}

export function resetStaticBackground(): void {
  staticState.mountains = [];
  staticState.trees = [];
  staticState.gradient = null;
  invalidateStaticCache();
}

export function ensureStaticLayer(
  canvas: HTMLCanvasElement,
  cssW: number,
  cssH: number,
  dpr: number,
): void {
  const physicalW = Math.max(1, Math.floor(cssW * dpr));
  const physicalH = Math.max(1, Math.floor(cssH * dpr));
  if (staticCanvas && staticCtx && staticCacheW === physicalW && staticCacheH === physicalH) return;
  if (!staticCanvas) staticCanvas = document.createElement('canvas');
  staticCanvas.width = physicalW;
  staticCanvas.height = physicalH;
  staticCanvas.style.width = `${cssW}px`;
  staticCanvas.style.height = `${cssH}px`;
  staticCtx = staticCanvas.getContext('2d');
  if (!staticCtx) return;
  staticCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
  initImages();
  const horizon = cssH * (1 - LAKE_RATIO);
  staticState.mountains = generateMountains(cssW, horizon);
  staticState.trees = [];
  for (let i = 0; i < 50; i++) {
    const r = Math.random();
    if (r < 0.35) {
      staticState.trees.push(Math.random() * 0.25);
    } else if (r > 0.65) {
      staticState.trees.push(0.75 + Math.random() * 0.25);
    } else {
      staticState.trees.push(0.25 + Math.random() * 0.5);
    }
  }
  staticState.gradient = createSkyGradient(staticCtx, cssH);
  drawSkyTo(staticCtx, cssW, cssH);
  drawMoonTo(staticCtx, cssW, cssH);
  drawMountainsTo(staticCtx, cssW, cssH);
  drawForestTo(staticCtx, cssW, cssH);
  drawLakeBaseTo(staticCtx, cssW, cssH);
  staticCacheW = physicalW;
  staticCacheH = physicalH;
}

export function getStaticCanvas(): HTMLCanvasElement | null {
  return staticCanvas;
}
