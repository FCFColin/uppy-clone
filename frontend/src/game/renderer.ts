import { getState, type ClientPlayer } from './state.js';
import { commitRenderedState } from './state_interp.js';
import {
  drawBalloon, drawBird, drawGhost, drawRipples, drawExplosion, pruneEffects,
} from './renderer_draw.js';
import {
  drawTutorialRangeCircle, drawDangerVignettes, drawFloatingTexts,
} from './visual_helpers.js';
import { drainPendingMessages } from './ws_connection.js';
import { registerResetFn } from './reset_registry.js';

interface CanvasLayout {
  top: number;
  bottom: number;
  width: number;
  height: number;
}

interface ImageEntry {
  img: HTMLImageElement;
  loaded: boolean;
  url: string;
  fallback: string;
}

export interface Star {
  x: number;
  y: number;
  size: number;
  twinkle: number;
}

export interface Cloud {
  x: number;
  y: number;
  width: number;
  speed: number;
  opacity: number;
  layer: number;
  variant: number;
  bobPhase: number;
}

export interface Mountain {
  x: number;
  height: number;
  width: number;
}

export interface Particle {
  x: number;
  y: number;
  size: number;
  life: number;
}

export interface BackgroundState {
  stars: Star[];
  clouds: Cloud[];
  gradient: CanvasGradient | null;
  mountains: Mountain[];
  particles: Particle[];
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

export const CLOUD_Y_MIN = 0.06;
export const CLOUD_Y_MAX = 0.52;

export const gameImages: Record<string, ImageEntry> = {
  sky:       { img: new Image(), loaded: false, url: '/assets/sky-bg.webp',      fallback: '/assets/fallback/sky-bg.svg' },
  cloud:     { img: new Image(), loaded: false, url: '/assets/cloud-1.webp',     fallback: '/assets/fallback/cloud-1.svg' },
  mountains: { img: new Image(), loaded: false, url: '/assets/mountains.webp',   fallback: '/assets/fallback/mountains.svg' },
  ghost:     { img: new Image(), loaded: false, url: '/assets/enemy-ghost.webp', fallback: '/assets/fallback/enemy-ghost.svg' },
  balloon:   { img: new Image(), loaded: false, url: '/assets/balloon-red.webp', fallback: '/assets/fallback/balloon-red.svg' },
  explosion: { img: new Image(), loaded: false, url: '/assets/explosion.webp',   fallback: '/assets/fallback/explosion.svg' },
};

export const cloudImages: ImageEntry[] = [
  { img: new Image(), loaded: false, url: '/assets/cloud-1.webp', fallback: '/assets/fallback/cloud-1.svg' },
  { img: new Image(), loaded: false, url: '/assets/cloud-2.webp', fallback: '/assets/fallback/cloud-2.svg' },
  { img: new Image(), loaded: false, url: '/assets/cloud-3.webp', fallback: '/assets/fallback/cloud-3.svg' },
];

export const bgState: BackgroundState = {
  stars: [],
  clouds: [],
  gradient: null,
  mountains: [],
  particles: [],
};

const CLOUD_LAYERS: CloudLayerConfig[] = [
  { count: 5, yMin: 0.08, yRange: 0.14, widthMin: 0.14, widthRange: 0.08, speedMin: 0.00008, speedRange: 0.00004, opacityMin: 0.28, opacityRange: 0.1, layer: 0 },
  { count: 4, yMin: 0.18, yRange: 0.16, widthMin: 0.11, widthRange: 0.07, speedMin: 0.00012, speedRange: 0.00006, opacityMin: 0.38, opacityRange: 0.1, layer: 1 },
  { count: 3, yMin: 0.28, yRange: 0.14, widthMin: 0.08, widthRange: 0.05, speedMin: 0.00018, speedRange: 0.00008, opacityMin: 0.48, opacityRange: 0.1, layer: 2 },
];

let layout: CanvasLayout = { top: 0, bottom: 0, width: 0, height: 0 };
let fallbackCanvas: HTMLCanvasElement | null = null;
let _ctx: CanvasRenderingContext2D | null = null;
let staticCanvas: HTMLCanvasElement | null = null;
let staticCtx: CanvasRenderingContext2D | null = null;
let staticCacheW = 0;
let staticCacheH = 0;
let renderActive = true;
let loopRunning = false;
let cachedPlayerMap: Map<number, ClientPlayer> | null = null;
let cachedPlayerMapKey: string | null = null;
let _previousTimestamp: number | undefined;

function measureLayoutInsets(): CanvasLayout {
  const hud = document.getElementById('game-hud');
  const cooldown = document.getElementById('cooldown-indicator');
  let top = 0;
  let bottom = 0;
  if (hud && !hud.classList.contains('hidden')) {
    const hudTop = hud.querySelector('.hud-top') as HTMLElement | null;
    top = hudTop ? hudTop.offsetHeight : 0;
    const hudBottom = hud.querySelector('.hud-bottom') as HTMLElement | null;
    if (hudBottom) { bottom = Math.max(bottom, hudBottom.offsetHeight); }
  }
  if (cooldown && !cooldown.classList.contains('hidden')) {
    bottom = Math.max(bottom, cooldown.offsetHeight + 12);
  }
  layout = { top, bottom, width: window.innerWidth, height: Math.max(1, window.innerHeight - top - bottom) };
  return layout;
}

export const $canvas = (document.getElementById('game-canvas') ?? (() => {
  fallbackCanvas = document.createElement('canvas');
  return fallbackCanvas;
})()) as HTMLCanvasElement;

export function getCtx(): CanvasRenderingContext2D {
  if (!_ctx) {
    _ctx = $canvas.getContext('2d');
    if (!_ctx) throw new Error('game canvas 2d context unavailable');
  }
  return _ctx;
}

export function resizeCanvas(): void {
  measureLayoutInsets();
  $canvas.width = layout.width;
  $canvas.height = layout.height;
  _ctx = null;
  if (document.getElementById('game-canvas')) {
    $canvas.style.top = `${layout.top}px`;
    $canvas.style.bottom = `${layout.bottom}px`;
    $canvas.style.height = `${layout.height}px`;
  }
  refreshBackgroundGradient();
  invalidateBackgroundStaticCache();
}
export function clientToNormalized(clientX: number, clientY: number): { x: number; y: number } {
  const rect = $canvas.getBoundingClientRect();
  const w = rect.width || 1;
  const h = rect.height || 1;
  return { x: (clientX - rect.left) / w, y: 1 - (clientY - rect.top) / h };
}

function loadImageEntry(entry: ImageEntry, cacheKey: string): void {
  entry.img.onload = () => {
    entry.loaded = true;
    if (cacheKey === 'sky' || cacheKey === 'mountains') {
      invalidateBackgroundStaticCache();
    }
  };
  entry.img.onerror = () => {
    entry.img.onerror = null;
    entry.img.src = entry.fallback;
  };
  entry.img.src = entry.url;
}

export function initImages(): void {
  for (const key in gameImages) {
    loadImageEntry(gameImages[key]!, key);
  }
  for (const entry of cloudImages) {
    loadImageEntry(entry, entry.url);
  }
}

export function randomCloudY(): number {
  return CLOUD_Y_MIN + Math.random() * (CLOUD_Y_MAX - CLOUD_Y_MIN);
}

export function spawnCloud(layerCfg: CloudLayerConfig, x?: number): Cloud {
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

export function initBackground(): void {
  initImages();
  bgState.stars = Array.from({ length: 60 }, () => ({
    x: Math.random(), y: Math.random() * 0.55,
    size: Math.random() * 2 + 1.2, twinkle: Math.random() * Math.PI * 2,
  }));
  bgState.clouds = CLOUD_LAYERS.flatMap(cfg =>
    Array.from({ length: cfg.count }, () => spawnCloud(cfg)),
  );
  bgState.mountains = Array.from({ length: 5 }, (_, i) => ({
    x: i * 0.25 - 0.05, height: 0.15 + Math.random() * 0.1, width: 0.3,
  }));
  bgState.particles = Array.from({ length: 20 }, () => ({
    x: Math.random(), y: Math.random() * 0.8, size: 0.5 + Math.random() * 1, life: Math.random(),
  }));
  bgState.gradient = getCtx().createLinearGradient(0, 0, 0, $canvas.height);
  bgState.gradient.addColorStop(0, '#0f1729');
  bgState.gradient.addColorStop(0.5, '#16213e');
  bgState.gradient.addColorStop(1, '#1a1a2e');
}

function refreshBackgroundGradient(): void {
  bgState.gradient = getCtx().createLinearGradient(0, 0, 0, $canvas.height);
  bgState.gradient.addColorStop(0, '#0f1729');
  bgState.gradient.addColorStop(0.5, '#16213e');
  bgState.gradient.addColorStop(1, '#1a1a2e');
}

export function ensureBackgroundInitialized(): void {
  if (bgState.clouds.length === 0 || bgState.stars.length === 0 || !bgState.gradient) {
    initBackground();
  }
}

export function invalidateBackgroundStaticCache(): void {
  staticCacheW = 0;
  staticCacheH = 0;
}
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
  const ellipses: [number, number, number, number][] = [
    [-0.24, 0.12, 0.3, 0.34],
    [0, -0.06, 0.34, 0.4],
    [0.26, 0.08, 0.28, 0.32],
  ];
  target.fillStyle = `rgba(90, 130, 170, ${alpha * 0.12})`;
  target.beginPath();
  for (const [dx, dy, rx, ry] of ellipses) {
    target.ellipse(cx + dx * w + (dx < 0 ? 4 : dx > 0 ? 3 : 3), cy + dy * h + 5, w * rx, h * ry, 0, 0, Math.PI * 2);
  }
  target.fill();
  target.fillStyle = `rgba(255, 255, 255, ${alpha})`;
  target.beginPath();
  for (const [dx, dy, rx, ry] of ellipses) {
    target.ellipse(cx + dx * w, cy + dy * h, w * rx, h * ry, 0, 0, Math.PI * 2);
  }
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
    const drawHeight = Math.min(w * (img.height / img.width), h * 0.4);
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
  if (staticCanvas && staticCtx && staticCacheW === w && staticCacheH === h) return;
  if (!staticCanvas) { staticCanvas = document.createElement('canvas'); }
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
  if (staticCanvas) { getCtx().drawImage(staticCanvas, 0, 0); }
  const time = now * 0.001;
  const windDir = getState().wind || 0;
  drawStars(time);
  drawCloudLayer(time, windDir);
  drawParticles(windDir);
}
export function setRenderActive(active: boolean): void {
  renderActive = active;
}

function getPlayerMap(): Map<number, ClientPlayer> {
  const players = getState().players;
  const key = players.map(p =>
    `${p.playerIndex}:${p.scoreContribution}:${p.nickname}:${p.palette}`
  ).join('|');
  if (key === cachedPlayerMapKey && cachedPlayerMap !== null) {
    return cachedPlayerMap;
  }
  cachedPlayerMapKey = key;
  cachedPlayerMap = new Map(players.map(p => [p.playerIndex, p]));
  return cachedPlayerMap;
}

export function renderOnce(): void {
  render();
}

export function startGameLoop(): void {
  if (loopRunning) return;
  loopRunning = true;
  requestAnimationFrame(gameLoop);
}

export function gameLoop(timestamp: number): void {
  if (!renderActive) {
    requestAnimationFrame(gameLoop);
    return;
  }
  if (_previousTimestamp !== undefined) {
    const delta = timestamp - _previousTimestamp;
    if (delta > 33) {
      console.warn(`Frame budget exceeded: ${delta.toFixed(1)}ms (target: 16.7ms for 60fps)`);
    }
  }
  _previousTimestamp = timestamp;
  try {
    drainPendingMessages(8);
  } catch (err: unknown) {
    console.error('drainPendingMessages error:', err);
  }
  render();
  requestAnimationFrame(gameLoop);
}

function render(): void {
  try {
    const now = Date.now();
    getCtx().fillStyle = '#1a1a2e';
    getCtx().fillRect(0, 0, $canvas.width, $canvas.height);
    drawBackground(now);
    if (getState().blockGameRender) return;
    if (getState().phase !== 'playing' && getState().phase !== 'ended') return;
    if (getState().hasReceivedFirstSnapshot) {
      drawTutorialRangeCircle(now);
      drawBalloon(now);
      drawBird(now);
      drawGhost(now);
      drawDangerVignettes(now);
      if (getState().phase === 'playing') {
        commitRenderedState(now);
      }
    }
    const playerMap = getPlayerMap();
    pruneEffects();
    drawRipples(now, playerMap);
    drawFloatingTexts(now);
    drawExplosion(now);
  } catch (err: unknown) {
    console.error('Render error:', err);
  }
}

export function resetRendererState(): void {
  cachedPlayerMap = null;
  cachedPlayerMapKey = null;
  invalidateBackgroundStaticCache();
}

registerResetFn(resetRendererState);