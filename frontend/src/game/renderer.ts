import { dispatch, getState, type ClientPlayer } from './state.js';
import { commitRenderedState, getInterpolatedBalloon, getInterpolatedBird, getInterpolatedGhost } from './state_interp.js';
import { registerResetFn } from './reset_registry.js';
import {
  drawTutorialRangeCircle, drawDangerVignettes, drawFloatingTexts,
  drawRadialGlow,
} from './visual_helpers.js';
import { drawBackground, invalidateBackgroundCache, resetBackgroundState } from './background_renderer.js';
import { drainPendingMessages } from './ws_connection.js';
import { PALETTE_COLORS } from '../shared/game/constants.js';

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

export interface TrailParticle {
  x: number;
  y: number;
  life: number;
  size: number;
  angle?: number;
  tailLength?: number;
}

export interface ExplosionParticle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  size: number;
  life: number;
  maxLife: number;
  color: string;
  type: 'spark' | 'smoke' | 'ember';
}

export const gameImages: Record<string, ImageEntry> = {
  ghost:     { img: new Image(), loaded: false, url: '/assets/enemy-ghost.webp', fallback: '/assets/fallback/enemy-ghost.svg' },
  balloon:   { img: new Image(), loaded: false, url: '/assets/balloon-red.webp', fallback: '/assets/fallback/balloon-red.svg' },
  explosion: { img: new Image(), loaded: false, url: '/assets/explosion.webp',   fallback: '/assets/fallback/explosion.svg' },
};

let layout: CanvasLayout = { top: 0, bottom: 0, width: 0, height: 0 };
let fallbackCanvas: HTMLCanvasElement | null = null;
let _ctx: CanvasRenderingContext2D | null = null;
let renderActive = true;
let loopRunning = false;
let cachedPlayerMap: Map<number, ClientPlayer> | null = null;
let cachedPlayerMapKey: string | null = null;
let _previousTimestamp: number | undefined;
let trailParticles: TrailParticle[] = [];
let trailFrameCounter = 0;
let explosionParticles: ExplosionParticle[] = [];
let lastExplosionId: string | null = null;

const MAX_DEVICE_PIXEL_RATIO = 2;
const MAX_PHYSICAL_PIXELS = 1920;
let cssCanvasWidth = 0;
let cssCanvasHeight = 0;
let devicePixelRatioValue = 1;

export function getCssCanvasSize(): { width: number; height: number } {
  return { width: cssCanvasWidth, height: cssCanvasHeight };
}

export function getDevicePixelRatio(): number {
  return devicePixelRatioValue;
}

let _prefersReducedMotion: boolean | null = null;
function prefersReducedMotion(): boolean {
  if (_prefersReducedMotion === null) {
    _prefersReducedMotion =
      typeof window !== 'undefined' && typeof window.matchMedia === 'function'
        ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
        : false;
    if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
      const mql = window.matchMedia('(prefers-reduced-motion: reduce)');
      mql.addEventListener?.('change', (e) => {
        _prefersReducedMotion = e.matches;
      });
    }
  }
  return _prefersReducedMotion;
}

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
    _ctx.setTransform(devicePixelRatioValue, 0, 0, devicePixelRatioValue, 0, 0);
  }
  return _ctx;
}

export function resizeCanvas(): void {
  measureLayoutInsets();
  cssCanvasWidth = Math.max(1, Math.floor(layout.width));
  cssCanvasHeight = Math.max(1, Math.floor(layout.height));
  const rawDpr = typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1;
  const dprCap = Math.min(rawDpr, MAX_DEVICE_PIXEL_RATIO);
  devicePixelRatioValue = Math.max(1, Math.min(dprCap, MAX_PHYSICAL_PIXELS / Math.max(1, cssCanvasWidth)));
  $canvas.width = Math.max(1, Math.floor(cssCanvasWidth * devicePixelRatioValue));
  $canvas.height = Math.max(1, Math.floor(cssCanvasHeight * devicePixelRatioValue));
  _ctx = null;
  if (document.getElementById('game-canvas')) {
    $canvas.style.position = 'fixed';
    $canvas.style.left = '0';
    $canvas.style.right = '0';
    $canvas.style.top = `${layout.top}px`;
    $canvas.style.bottom = `${layout.bottom}px`;
    $canvas.style.width = `${cssCanvasWidth}px`;
    $canvas.style.height = `${cssCanvasHeight}px`;
  }
  invalidateBackgroundCache();
}
export function clientToNormalized(clientX: number, clientY: number): { x: number; y: number } {
  const rect = $canvas.getBoundingClientRect();
  const w = rect.width || 1;
  const h = rect.height || 1;
  return { x: (clientX - rect.left) / w, y: 1 - (clientY - rect.top) / h };
}

function loadImageEntry(entry: ImageEntry): void {
  entry.img.onload = () => {
    entry.loaded = true;
  };
  entry.img.onerror = () => {
    entry.img.onerror = null;
    entry.img.src = entry.fallback;
  };
  entry.img.src = entry.url;
}

function initImages(): void {
  for (const key in gameImages) {
    loadImageEntry(gameImages[key]!);
  }
}

initImages();

function updateAndDrawTrailParticles(): void {
  const ctx = getCtx();
  const w = cssCanvasWidth;
  const h = cssCanvasHeight;
  for (let i = trailParticles.length - 1; i >= 0; i--) {
    const p = trailParticles[i]!;
    p.life -= 0.022;
    p.size += 0.2;
    if (p.life <= 0) {
      trailParticles.splice(i, 1);
      continue;
    }
    const px = p.x * w;
    const py = (1 - p.y) * h;
    const alpha = p.life;
    const glowSize = p.size * (1.8 + (1 - p.life) * 2);

    ctx.save();
    ctx.globalCompositeOperation = 'screen';
    const outerGlow = ctx.createRadialGradient(px, py, 0, px, py, glowSize);
    outerGlow.addColorStop(0, `rgba(80, 220, 255, ${alpha * 0.4})`);
    outerGlow.addColorStop(0.5, `rgba(100, 180, 255, ${alpha * 0.2})`);
    outerGlow.addColorStop(1, 'rgba(50, 150, 255, 0)');
    ctx.fillStyle = outerGlow;
    ctx.beginPath();
    ctx.arc(px, py, glowSize, 0, Math.PI * 2);
    ctx.fill();

    const innerGlow = ctx.createRadialGradient(px, py, 0, px, py, p.size);
    innerGlow.addColorStop(0, `rgba(200, 250, 255, ${alpha * 0.9})`);
    innerGlow.addColorStop(0.4, `rgba(100, 230, 255, ${alpha * 0.6})`);
    innerGlow.addColorStop(1, `rgba(60, 180, 255, ${alpha * 0.2})`);
    ctx.fillStyle = innerGlow;
    ctx.beginPath();
    ctx.arc(px, py, p.size, 0, Math.PI * 2);
    ctx.fill();

    if (p.angle !== undefined && p.tailLength) {
      const tailLenPx = p.tailLength * w;
      const tx = px - Math.cos(p.angle) * tailLenPx * (1 + (1 - p.life) * 2);
      const ty = py + Math.sin(p.angle) * tailLenPx * (1 + (1 - p.life) * 2);
      const tailGrad = ctx.createLinearGradient(px, py, tx, ty);
      tailGrad.addColorStop(0, `rgba(160, 240, 255, ${alpha * 0.7})`);
      tailGrad.addColorStop(1, 'rgba(80, 180, 255, 0)');
      ctx.strokeStyle = tailGrad;
      ctx.lineWidth = p.size * 0.5;
      ctx.lineCap = 'round';
      ctx.beginPath();
      ctx.moveTo(px, py);
      ctx.lineTo(tx, ty);
      ctx.stroke();
    }
    ctx.restore();
  }
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

let fpsMonitorInitialized = false;
let fpsMonitorEnabled = false;
let fpsOverlay: HTMLElement | null = null;
let fpsFrameCount = 0;
let fpsAccumulatedTime = 0;

function initFpsMonitor(): void {
  if (fpsMonitorInitialized) return;
  fpsMonitorInitialized = true;
  if (typeof window === 'undefined') return;
  try {
    if (import.meta.env.DEV) fpsMonitorEnabled = true;
    if (localStorage.getItem('uppy-debug-fps') === '1') fpsMonitorEnabled = true;
    if (new URLSearchParams(window.location.search).get('debug') === 'fps') fpsMonitorEnabled = true;
  } catch {
    // ignore — storage may be unavailable (private browsing etc.)
  }
  if (!fpsMonitorEnabled) return;
  fpsOverlay = document.createElement('div');
  fpsOverlay.style.cssText =
    'position:fixed;top:calc(6px + env(safe-area-inset-top,0px));left:6px;z-index:99999;' +
    'font:12px/1.4 ui-monospace,monospace;color:#4ade80;background:rgba(2,6,23,0.72);' +
    'padding:4px 8px;border-radius:6px;pointer-events:none;backdrop-filter:blur(4px);';
  document.body.appendChild(fpsOverlay);
}

function updateFpsMonitor(delta: number): void {
  if (!fpsMonitorEnabled) return;
  fpsFrameCount++;
  fpsAccumulatedTime += delta;
  if (fpsAccumulatedTime >= 1000) {
    const fps = Math.round((fpsFrameCount * 1000) / fpsAccumulatedTime);
    const ms = (fpsAccumulatedTime / fpsFrameCount).toFixed(1);
    if (fpsOverlay) fpsOverlay.textContent = `FPS ${fps} | ${ms}ms`;
    fpsFrameCount = 0;
    fpsAccumulatedTime = 0;
  }
}

export function renderOnce(): void {
  render();
}

export function startGameLoop(): void {
  if (loopRunning) return;
  loopRunning = true;
  initFpsMonitor();
  requestAnimationFrame(gameLoop);
}

export function gameLoop(timestamp: number): void {
  if (!renderActive) {
    requestAnimationFrame(gameLoop);
    return;
  }
  if (_previousTimestamp !== undefined) {
    const delta = timestamp - _previousTimestamp;
    updateFpsMonitor(delta);
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
    const ctx = getCtx();
    drawBackground(ctx, $canvas, now, getState().wind || 0);
    updateAndDrawTrailParticles();
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
  trailParticles = [];
  trailFrameCounter = 0;
  explosionParticles = [];
  lastExplosionId = null;
  _cachedBalloonRadius = 0;
  _balloonBodyGrad = null;
  _balloonRimGrad = null;
  _cachedGhostRadius = 0;
  _ghostBodyGrad = null;
  resetBackgroundState();
}

let _cachedBirdSize = 0;
let _wingGradTop: CanvasGradient | null = null;
let _wingGradBottom: CanvasGradient | null = null;
let _bodyGrad: CanvasGradient | null = null;
let _tailGrad: CanvasGradient | null = null;

let _cachedBalloonRadius = 0;
let _balloonBodyGrad: CanvasGradient | null = null;
let _balloonRimGrad: CanvasGradient | null = null;

let _cachedGhostRadius = 0;
let _ghostBodyGrad: CanvasGradient | null = null;

function _ensureBalloonGradients(ctx: CanvasRenderingContext2D, radius: number): void {
  if (_cachedBalloonRadius === radius && _balloonBodyGrad && _balloonRimGrad) return;
  _cachedBalloonRadius = radius;
  _balloonBodyGrad = ctx.createRadialGradient(
    -radius * 0.38,
    -radius * 0.4,
    radius * 0.05,
    0,
    0,
    radius * 1.05,
  );
  _balloonBodyGrad.addColorStop(0, '#ffd0dc');
  _balloonBodyGrad.addColorStop(0.25, '#ff9eb5');
  _balloonBodyGrad.addColorStop(0.55, '#ff6b8b');
  _balloonBodyGrad.addColorStop(0.82, '#d44060');
  _balloonBodyGrad.addColorStop(1, '#a82848');

  _balloonRimGrad = ctx.createRadialGradient(
    radius * 0.35,
    radius * 0.35,
    0,
    radius * 0.35,
    radius * 0.35,
    radius * 0.85,
  );
  _balloonRimGrad.addColorStop(0, 'rgba(255, 220, 230, 0.35)');
  _balloonRimGrad.addColorStop(0.5, 'rgba(255, 160, 180, 0.15)');
  _balloonRimGrad.addColorStop(1, 'rgba(255, 120, 150, 0)');
}

function _ensureGhostGradient(ctx: CanvasRenderingContext2D, radius: number): void {
  if (_cachedGhostRadius === radius && _ghostBodyGrad) return;
  _cachedGhostRadius = radius;
  _ghostBodyGrad = ctx.createRadialGradient(-radius * 0.3, -radius * 0.3, 0, 0, 0, radius * 1.2);
  _ghostBodyGrad.addColorStop(0, 'rgba(240, 235, 250, 0.9)');
  _ghostBodyGrad.addColorStop(0.5, 'rgba(200, 190, 230, 0.6)');
  _ghostBodyGrad.addColorStop(1, 'rgba(150, 140, 190, 0.2)');
}

function _ensureBirdGradients(ctx: CanvasRenderingContext2D, size: number): void {
  if (_cachedBirdSize === size && _wingGradTop && _wingGradBottom && _bodyGrad && _tailGrad) return;
  _cachedBirdSize = size;
  _wingGradTop = ctx.createLinearGradient(0, -size * 0.5, 0, size * 0.1);
  _wingGradTop.addColorStop(0, '#ffd700');
  _wingGradTop.addColorStop(0.3, '#ff9500');
  _wingGradTop.addColorStop(0.7, '#f45c00');
  _wingGradTop.addColorStop(1, '#c92a00');
  _wingGradBottom = ctx.createLinearGradient(0, size * 0.1, 0, size * 0.4);
  _wingGradBottom.addColorStop(0, '#ff9500');
  _wingGradBottom.addColorStop(1, '#a02000');
  _bodyGrad = ctx.createRadialGradient(-size * 0.2, -size * 0.2, size * 0.05, 0, 0, size * 0.7);
  _bodyGrad.addColorStop(0, '#fff4c0');
  _bodyGrad.addColorStop(0.2, '#ffd000');
  _bodyGrad.addColorStop(0.5, '#ff8800');
  _bodyGrad.addColorStop(0.8, '#e04400');
  _bodyGrad.addColorStop(1, '#b82800');
  _tailGrad = ctx.createLinearGradient(-size * 0.9, 0, -size * 0.4, 0);
  _tailGrad.addColorStop(0, '#8b1a00');
  _tailGrad.addColorStop(0.5, '#d03800');
  _tailGrad.addColorStop(1, '#ff8800');
}

export function drawBalloon(now: number = Date.now()): void {
  const interp = getInterpolatedBalloon(now);
  const bx = interp.x * cssCanvasWidth;
  const by = (1 - interp.y) * cssCanvasHeight;
  const baseRadius = Math.min(cssCanvasWidth, cssCanvasHeight) * 0.06;
  const reduced = prefersReducedMotion();
  const breath = reduced ? 1 : 1 + Math.sin(now * 0.0025) * 0.03;
  const radius = baseRadius * breath;
  const ctx = getCtx();
  const wind = getState().wind;
  const vx = getState().balloon.vx;

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  drawRadialGlow(ctx, bx, by, radius * 5.5, 'rgba(255, 160, 190, 0.22)', 'rgba(255, 160, 190, 0)');
  drawRadialGlow(ctx, bx, by, radius * 3.8, 'rgba(255, 130, 170, 0.32)', 'rgba(255, 130, 170, 0)');
  drawRadialGlow(ctx, bx, by, radius * 2.4, 'rgba(255, 190, 210, 0.28)', 'rgba(255, 190, 210, 0)');
  drawRadialGlow(ctx, bx, by, radius * 1.6, 'rgba(255, 220, 230, 0.18)', 'rgba(255, 220, 230, 0)');
  ctx.restore();

  const idleSway = reduced ? 0 : Math.sin(now * 0.0015) * 0.035;
  const velocityTilt = Math.atan2(vx, 3.5) * 0.55;
  const tilt = wind * 0.35 + velocityTilt + idleSway;

  const balloonImg = gameImages['balloon'];
  ctx.save();
  ctx.translate(bx, by);
  ctx.rotate(tilt);

  if (balloonImg && balloonImg.loaded) {
    const img: HTMLImageElement = balloonImg.img;
    const w = radius * 2.5;
    const h = w * (img.height / img.width);
    ctx.drawImage(img, -w / 2, -h / 2, w, h);
  } else {
    _ensureBalloonGradients(ctx, radius);
    ctx.beginPath();
    ctx.arc(0, 0, radius, 0, Math.PI * 2);
    ctx.fillStyle = _balloonBodyGrad!;
    ctx.fill();

    ctx.beginPath();
    ctx.arc(0, 0, radius, 0, Math.PI * 2);
    ctx.fillStyle = _balloonRimGrad!;
    ctx.globalCompositeOperation = 'screen';
    ctx.fill();
    ctx.globalCompositeOperation = 'source-over';

    ctx.beginPath();
    ctx.ellipse(-radius * 0.3, -radius * 0.35, radius * 0.26, radius * 0.18, -0.45, 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(255,255,255,0.72)';
    ctx.fill();

    ctx.beginPath();
    ctx.ellipse(-radius * 0.14, -radius * 0.2, radius * 0.1, radius * 0.07, -0.35, 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(255,255,255,0.38)';
    ctx.fill();

    ctx.beginPath();
    ctx.moveTo(-radius * 0.08, radius * 0.92);
    ctx.lineTo(radius * 0.08, radius * 0.92);
    ctx.lineTo(0, radius * 1.08);
    ctx.closePath();
    ctx.fillStyle = '#d44060';
    ctx.fill();
  }

  ctx.restore();

  if (!reduced) {
    const stringBaseX = bx + Math.sin(tilt) * radius * 1.05;
    const stringBaseY = by + Math.cos(tilt) * radius * 1.05;
    const stringLen = radius * 2.2;
    const swayFreq = now * 0.0025;
    const windSway = wind * 0.15 + Math.sin(swayFreq) * 0.04 + Math.sin(swayFreq * 1.7) * 0.02;
    const tipX = stringBaseX + windSway * stringLen;
    const tipY = stringBaseY + stringLen;
    const cpX = stringBaseX + windSway * stringLen * 0.5;
    const cpY = stringBaseY + stringLen * 0.5;

    ctx.save();
    ctx.strokeStyle = 'rgba(255, 240, 245, 0.55)';
    ctx.lineWidth = 1.5;
    ctx.lineCap = 'round';
    ctx.shadowBlur = 4;
    ctx.shadowColor = 'rgba(255, 180, 200, 0.25)';
    ctx.beginPath();
    ctx.moveTo(stringBaseX, stringBaseY);
    ctx.quadraticCurveTo(cpX, cpY, tipX, tipY);
    ctx.stroke();
    ctx.restore();
  }
}

function drawBirdTail(ctx: CanvasRenderingContext2D, size: number, flapPhase: number): void {
  ctx.save();
  const tailSpread = 1 + flapPhase * 0.3;
  ctx.beginPath();
  ctx.moveTo(-size * 0.45, 0);
  ctx.quadraticCurveTo(-size * 0.65, -size * 0.22 * tailSpread, -size * 0.9, -size * 0.25 * tailSpread);
  ctx.lineTo(-size * 0.82, -size * 0.05);
  ctx.quadraticCurveTo(-size * 0.85, 0, -size * 0.82, size * 0.05);
  ctx.lineTo(-size * 0.9, size * 0.25 * tailSpread);
  ctx.quadraticCurveTo(-size * 0.65, size * 0.22 * tailSpread, -size * 0.45, 0);
  ctx.closePath();
  ctx.fillStyle = _tailGrad!;
  ctx.fill();
  ctx.restore();
}

function drawBirdBeak(ctx: CanvasRenderingContext2D, size: number): void {
  ctx.beginPath();
  ctx.moveTo(size * 0.52, -size * 0.06);
  ctx.quadraticCurveTo(size * 0.78, -size * 0.02, size * 0.82, size * 0.02);
  ctx.quadraticCurveTo(size * 0.78, size * 0.08, size * 0.52, size * 0.12);
  ctx.quadraticCurveTo(size * 0.5, size * 0.03, size * 0.52, -size * 0.06);
  ctx.closePath();
  const beakGrad = ctx.createLinearGradient(size * 0.52, -size * 0.06, size * 0.82, size * 0.08);
  beakGrad.addColorStop(0, '#ffd000');
  beakGrad.addColorStop(0.5, '#ff9500');
  beakGrad.addColorStop(1, '#e06000');
  ctx.fillStyle = beakGrad;
  ctx.fill();
  ctx.beginPath();
  ctx.moveTo(size * 0.55, size * 0.03);
  ctx.lineTo(size * 0.75, size * 0.04);
  ctx.lineTo(size * 0.55, size * 0.08);
  ctx.fillStyle = 'rgba(180, 80, 0, 0.5)';
  ctx.fill();
}

function drawBirdEyes(ctx: CanvasRenderingContext2D, size: number): void {
  const eyeX = size * 0.3;
  const eyeY = -size * 0.13;
  ctx.beginPath();
  ctx.ellipse(eyeX, eyeY, size * 0.11, size * 0.12, 0, 0, Math.PI * 2);
  ctx.fillStyle = '#fff';
  ctx.fill();
  ctx.beginPath();
  ctx.arc(eyeX + size * 0.02, eyeY + size * 0.01, size * 0.06, 0, Math.PI * 2);
  ctx.fillStyle = '#1a1a1a';
  ctx.fill();
  ctx.beginPath();
  ctx.arc(eyeX - size * 0.02, eyeY - size * 0.03, size * 0.025, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(255,255,255,0.8)';
  ctx.fill();
}

export function drawBird(now: number): void {
  const bird = getInterpolatedBird(now);
  if (!bird || !bird.active) {
    trailFrameCounter = 0;
    return;
  }
  const bx: number = bird.x * cssCanvasWidth;
  const by: number = (1 - bird.y) * cssCanvasHeight;
  const size: number = Math.min(cssCanvasWidth, cssCanvasHeight) * 0.035;
  const reduced = prefersReducedMotion();

  const vx = getState().balloon.x - bird.x;
  const dir = vx >= 0 ? 1 : -1;

  const flapSpeed = reduced ? 0 : 0.012;
  const flapPhase = Math.sin(now * flapSpeed);
  const flapPhase2 = Math.sin(now * flapSpeed + 0.3);
  const wingY = reduced ? 0 : -Math.abs(flapPhase) * size * 0.65 + size * 0.1;
  const wingRot = reduced ? 0 : flapPhase * 0.8;
  const bodyBob = reduced ? 0 : Math.sin(now * 0.008) * size * 0.05;

  if (!reduced) {
    trailFrameCounter++;
    if (trailFrameCounter >= 3) {
      trailFrameCounter = 0;
      const offsetX = -dir * (0.025 + Math.random() * 0.02);
      const offsetY = (Math.random() - 0.5) * 0.02;
      trailParticles.push({
        x: bird.x + offsetX,
        y: bird.y + offsetY,
        life: 1.0,
        size: 3 + Math.random() * 3,
        angle: dir > 0 ? 0 : Math.PI,
        tailLength: 0.02 + Math.random() * 0.015,
      });
      if (trailParticles.length > 20) {
        trailParticles.shift();
      }
    }
  }

  const ctx = getCtx();
  _ensureBirdGradients(ctx, size);

  ctx.save();
  ctx.translate(bx, by + bodyBob);
  ctx.scale(dir, 1);

  ctx.save();
  ctx.translate(-size * 0.1, size * 0.05);
  ctx.rotate(-0.2 + wingRot * 0.5);
  ctx.beginPath();
  ctx.ellipse(0, wingY * 0.3, size * 0.5, size * 0.22, -0.5, 0, Math.PI * 2);
  ctx.fillStyle = _wingGradBottom!;
  ctx.fill();
  ctx.restore();

  ctx.save();
  ctx.translate(-size * 0.05, 0);
  ctx.rotate(-0.15 + wingRot);
  ctx.beginPath();
  ctx.ellipse(0, wingY, size * 0.58, size * 0.25, -0.4 + wingRot * 0.3, 0, Math.PI * 2);
  ctx.fillStyle = _wingGradTop!;
  ctx.fill();
  ctx.beginPath();
  ctx.ellipse(-size * 0.05, wingY - size * 0.08, size * 0.25, size * 0.12, -0.3, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(255, 220, 150, 0.3)';
  ctx.fill();
  ctx.restore();

  ctx.beginPath();
  ctx.ellipse(0, 0, size * 0.58, size * 0.4, 0, 0, Math.PI * 2);
  ctx.fillStyle = _bodyGrad!;
  ctx.fill();

  ctx.beginPath();
  ctx.ellipse(-size * 0.25, size * 0.05, size * 0.22, size * 0.15, -0.3, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(255, 200, 100, 0.25)';
  ctx.fill();

  ctx.save();
  ctx.translate(size * 0.05, 0);
  ctx.rotate(0.15 - wingRot * 0.6);
  ctx.beginPath();
  ctx.ellipse(0, wingY * 0.8, size * 0.48, size * 0.2, 0.4 - wingRot * 0.3, 0, Math.PI * 2);
  ctx.fillStyle = _wingGradTop!;
  ctx.fill();
  ctx.restore();

  drawBirdTail(ctx, size, flapPhase2);
  drawBirdBeak(ctx, size);
  drawBirdEyes(ctx, size);

  ctx.restore();
}

export function drawGhost(now: number): void {
  const interpGhost = getInterpolatedGhost(now);
  if (!interpGhost) return;
  const radius: number = Math.min(cssCanvasWidth, cssCanvasHeight) * 0.038;
  const gx = interpGhost.x * cssCanvasWidth;
  const reduced = prefersReducedMotion();
  const floatBob = reduced ? 0 : Math.sin(now * 0.002) * radius * 0.18;
  const floatSway = reduced ? 0 : Math.sin(now * 0.0014) * radius * 0.1;
  const gy = (1 - interpGhost.y) * cssCanvasHeight + floatBob;
  const drawX = gx + floatSway;

  const repelTimer = getState().ghost.repelTimer;
  const isRepelled = repelTimer > 0;
  const ctx = getCtx();
  const ghostImg = gameImages['ghost'];

  if (isRepelled) {
    const repelProgress = 1 - repelTimer / 500;
    const shockwaveRadius = radius * (2 + repelProgress * 6);
    const shockAlpha = (1 - repelProgress) * 0.4;

    ctx.save();
    ctx.globalCompositeOperation = 'screen';
    for (let i = 0; i < 4; i++) {
      const waveR = shockwaveRadius * (0.5 + i * 0.25);
      const waveA = shockAlpha * (1 - i * 0.2);
      const shockGrad = ctx.createRadialGradient(drawX, gy, waveR * 0.8, drawX, gy, waveR);
      shockGrad.addColorStop(0, 'rgba(200, 100, 100, 0)');
      shockGrad.addColorStop(0.45, `rgba(200, 100, 100, ${waveA * 0.35})`);
      shockGrad.addColorStop(0.75, `rgba(200, 120, 100, ${waveA * 0.175})`);
      shockGrad.addColorStop(1, 'rgba(200, 80, 80, 0)');
      ctx.fillStyle = shockGrad;
      ctx.beginPath();
      ctx.arc(drawX, gy, waveR, 0, Math.PI * 2);
      ctx.fill();
    }
    drawRadialGlow(ctx, drawX, gy, radius * 2.5, `rgba(200, 80, 80, ${0.25 + Math.sin(now * 0.05) * 0.1})`, 'rgba(200, 80, 80, 0)');
    drawRadialGlow(ctx, drawX, gy, radius * 1.5, `rgba(200, 100, 90, ${0.35 + Math.sin(now * 0.06) * 0.1})`, 'rgba(200, 100, 90, 0)');
    ctx.restore();

    const size: number = radius * 4;
    if (ghostImg?.loaded) {
      const flashIntensity = 0.4 + Math.sin(now * 0.025) * 0.12;
      ctx.save();
      ctx.globalAlpha = flashIntensity;
      ctx.drawImage(ghostImg.img, drawX - size / 2, gy - size / 2, size, size);
      ctx.restore();
    } else {
      const flashIntensity = 0.5 + Math.sin(now * 0.03) * 0.15;
      _ensureGhostGradient(ctx, radius);
      ctx.fillStyle = _ghostBodyGrad!;
      ctx.globalAlpha = flashIntensity;
      ctx.beginPath();
      ctx.arc(drawX, gy, radius * 1.1, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1;

      ctx.fillStyle = `rgba(255, 255, 255, ${flashIntensity})`;
      ctx.beginPath();
      ctx.arc(drawX - radius * 0.3, gy - radius * 0.2, radius * 0.22, 0, Math.PI * 2);
      ctx.arc(drawX + radius * 0.3, gy - radius * 0.2, radius * 0.22, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = 'rgba(40, 30, 70, 1)';
      ctx.beginPath();
      ctx.arc(drawX - radius * 0.28, gy - radius * 0.18, radius * 0.12, 0, Math.PI * 2);
      ctx.arc(drawX + radius * 0.32, gy - radius * 0.18, radius * 0.12, 0, Math.PI * 2);
      ctx.fill();
    }
    return;
  }

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  drawRadialGlow(ctx, drawX, gy, radius * 2, 'rgba(180, 160, 220, 0.08)', 'rgba(180, 160, 220, 0)');
  ctx.restore();

  if (ghostImg && ghostImg.loaded) {
    const size: number = radius * 4;
    const alphaPulse = reduced ? 0.88 : 0.88 + Math.sin(now * 0.0025) * 0.08;
    ctx.save();
    ctx.globalAlpha = alphaPulse;
    ctx.drawImage(ghostImg.img, drawX - size / 2, gy - size / 2, size, size);
    ctx.restore();
    return;
  }

  const pulsePhase = Math.sin(now * 0.0025);
  const pulseAlpha = reduced ? 0.8 : 0.78 + pulsePhase * 0.08;
  const bodyScale = reduced ? 1 : 1 + pulsePhase * 0.03;
  const wispPhase = now * 0.0015;

  ctx.save();
  ctx.globalAlpha = pulseAlpha;
  ctx.beginPath();
  ctx.ellipse(drawX, gy - radius * 0.1, radius * 0.9 * bodyScale, radius * bodyScale, 0, Math.PI, 0);
  const wispCount = 5;
  for (let i = 0; i <= wispCount; i++) {
    const t = i / wispCount;
    const wx = drawX - radius * 0.9 * bodyScale + t * radius * 1.8 * bodyScale;
    const wispHeight = radius * (0.25 + Math.sin(wispPhase + i * 1.2) * 0.15);
    const wy = gy + radius * 0.1 + wispHeight;
    if (i === 0) {
      ctx.lineTo(wx, wy);
    } else {
      const cpx = drawX - radius * 0.9 * bodyScale + (t - 0.5 / wispCount) * radius * 1.8 * bodyScale;
      ctx.quadraticCurveTo(cpx, wy + radius * 0.1, wx, wy);
    }
  }
  ctx.lineTo(drawX + radius * 0.9 * bodyScale, gy + radius * 0.1);
  ctx.closePath();

  const bodyGrad: CanvasGradient = ctx.createRadialGradient(
    drawX - radius * 0.35,
    gy - radius * 0.4,
    0,
    drawX,
    gy,
    radius * 1.3
  );
  bodyGrad.addColorStop(0, 'rgba(220, 190, 255, 0.95)');
  bodyGrad.addColorStop(0.4, 'rgba(180, 130, 255, 0.8)');
  bodyGrad.addColorStop(0.75, 'rgba(150, 90, 230, 0.6)');
  bodyGrad.addColorStop(1, 'rgba(120, 60, 200, 0.25)');
  ctx.fillStyle = bodyGrad;
  ctx.fill();

  ctx.beginPath();
  ctx.ellipse(drawX - radius * 0.3, gy - radius * 0.5, radius * 0.28, radius * 0.18, -0.3, 0, Math.PI * 2);
  ctx.fillStyle = 'rgba(240, 225, 255, 0.35)';
  ctx.fill();
  ctx.restore();

  const eyePulse = 0.9 + Math.sin(now * 0.004) * 0.1;
  ctx.fillStyle = `rgba(255, 255, 255, ${eyePulse})`;
  ctx.beginPath();
  ctx.ellipse(drawX - radius * 0.28, gy - radius * 0.2, radius * 0.18, radius * 0.22, 0, 0, Math.PI * 2);
  ctx.ellipse(drawX + radius * 0.28, gy - radius * 0.2, radius * 0.18, radius * 0.22, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = 'rgba(30, 10, 60, 0.9)';
  ctx.beginPath();
  ctx.ellipse(drawX - radius * 0.26, gy - radius * 0.17, radius * 0.09, radius * 0.12, 0, 0, Math.PI * 2);
  ctx.ellipse(drawX + radius * 0.3, gy - radius * 0.17, radius * 0.09, radius * 0.12, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = 'rgba(255, 255, 255, 0.7)';
  ctx.beginPath();
  ctx.arc(drawX - radius * 0.3, gy - radius * 0.25, radius * 0.04, 0, Math.PI * 2);
  ctx.arc(drawX + radius * 0.26, gy - radius * 0.25, radius * 0.04, 0, Math.PI * 2);
  ctx.fill();
}

const RIPPLE_DURATION_S = 0.6;
const _rejectedRgb = 'rgba(233,69,96';
const _optimisticRgb = 'rgba(0,180,216';

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
  const reduced = prefersReducedMotion();
  for (const ripple of remaining) {
    const age = (now - ripple.time) / 1000;
    const t = Math.min(1, age / RIPPLE_DURATION_S);
    if (t >= 1) continue;

    const rx = ripple.x * cssCanvasWidth;
    const ry = (1 - ripple.y) * cssCanvasHeight;
    const maxRadius = Math.min(cssCanvasWidth, cssCanvasHeight) * 0.07;
    const radius = maxRadius * (0.25 + 0.75 * t);
    const alpha = (1 - t) * 0.9;

    if (ripple.rejected) {
      const s = 14 + 10 * t;
      const pulse = reduced ? 1 : 1 + Math.sin(now * 0.02) * 0.15;
      ctx.save();
      ctx.globalAlpha = alpha;
      ctx.strokeStyle = 'rgba(233, 69, 96, 1)';
      ctx.lineWidth = 3 * pulse;
      ctx.lineCap = 'round';
      ctx.shadowBlur = 10;
      ctx.shadowColor = 'rgba(233, 69, 96, 0.55)';
      ctx.beginPath();
      ctx.moveTo(rx - s, ry - s);
      ctx.lineTo(rx + s, ry + s);
      ctx.moveTo(rx + s, ry - s);
      ctx.lineTo(rx - s, ry + s);
      ctx.stroke();
      ctx.restore();
    } else {
      const { base } = rippleColor(ripple, playerMap);
      ctx.save();
      ctx.globalCompositeOperation = 'screen';
      ctx.globalAlpha = alpha * 0.35;
      ctx.fillStyle = base + ', 1)';
      ctx.beginPath();
      ctx.arc(rx, ry, radius * 1.4, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();

      ctx.save();
      ctx.globalAlpha = alpha;
      ctx.lineCap = 'round';

      ctx.beginPath();
      ctx.arc(rx, ry, radius, 0, Math.PI * 2);
      ctx.strokeStyle = base + ', 0.85)';
      ctx.lineWidth = 3;
      ctx.shadowBlur = 8;
      ctx.shadowColor = base + ', 0.45)';
      ctx.stroke();

      ctx.beginPath();
      ctx.arc(rx, ry, radius * 0.72, 0, Math.PI * 2);
      ctx.strokeStyle = base + ', 0.55)';
      ctx.lineWidth = 1.5;
      ctx.shadowBlur = 4;
      ctx.shadowColor = base + ', 0.35)';
      ctx.stroke();

      ctx.restore();
    }
    ctx.globalAlpha = 1;
  }
}

const EXPLOSION_COLORS = [
  'rgba(255, 220, 80,',
  'rgba(255, 160, 40,',
  'rgba(255, 90, 30,',
  'rgba(230, 50, 60,',
  'rgba(180, 40, 80,',
  'rgba(120, 50, 120,',
];

function spawnExplosionParticles(x: number, y: number): void {
  const reduced = prefersReducedMotion();
  const count = reduced ? 10 : 20;
  explosionParticles = [];
  for (let i = 0; i < count; i++) {
    const angle = Math.random() * Math.PI * 2;
    const speed = 0.002 + Math.random() * 0.004;
    const size = 2 + Math.random() * 5;
    const colorBase = EXPLOSION_COLORS[Math.floor(Math.random() * EXPLOSION_COLORS.length)] ?? 'rgba(255, 160, 40,';
    const typeRand = Math.random();
    const type: ExplosionParticle['type'] = typeRand > 0.7 ? 'smoke' : typeRand > 0.4 ? 'ember' : 'spark';
    explosionParticles.push({
      x,
      y,
      vx: Math.cos(angle) * speed * (type === 'smoke' ? 0.6 : 1),
      vy: Math.sin(angle) * speed * (type === 'smoke' ? 0.6 : 1),
      size,
      life: 1,
      maxLife: 0.5 + Math.random() * 0.5,
      color: colorBase,
      type,
    });
  }
}

export function drawExplosion(now: number): void {
  const explosion = getState().explosionEffect;
  if (!explosion) return;

  const elapsed = now - explosion.startTime;
  const duration = 600;
  if (elapsed > duration) return;
  const progress = Math.min(1, elapsed / duration);
  const ctx = getCtx();
  const ex = explosion.x * cssCanvasWidth;
  const ey = (1 - explosion.y) * cssCanvasHeight;
  const scale = Math.min(cssCanvasWidth, cssCanvasHeight);
  const baseSize = scale * 0.16;
  const reduced = prefersReducedMotion();

  const explosionId = `${explosion.x}-${explosion.y}-${explosion.startTime}`;
  if (lastExplosionId !== explosionId) {
    lastExplosionId = explosionId;
    spawnExplosionParticles(explosion.x, explosion.y);
  }

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  ctx.globalAlpha = (1 - progress) * 0.6;
  const shockRadius = baseSize * (0.2 + progress * 1.6);
  ctx.fillStyle = 'rgba(255, 180, 100, 1)';
  ctx.beginPath();
  ctx.arc(ex, ey, shockRadius, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  ctx.save();
  ctx.globalCompositeOperation = 'screen';
  ctx.globalAlpha = (1 - progress) * 0.45;
  ctx.fillStyle = 'rgba(255, 220, 140, 1)';
  ctx.beginPath();
  ctx.arc(ex, ey, baseSize * 0.6, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  if (gameImages['explosion']!.loaded) {
    const size = baseSize * (0.4 + progress * 0.8);
    ctx.save();
    ctx.globalAlpha = 1 - progress;
    ctx.globalCompositeOperation = 'screen';
    ctx.drawImage(gameImages['explosion']!.img, ex - size / 2, ey - size / 2, size, size);
    ctx.restore();
  }

  if (explosionParticles.length > 0) {
    ctx.save();
    ctx.globalCompositeOperation = reduced ? 'source-over' : 'screen';
    for (let i = explosionParticles.length - 1; i >= 0; i--) {
      const p = explosionParticles[i]!;
      p.life -= 0.016;
      if (p.life <= 0) {
        explosionParticles.splice(i, 1);
        continue;
      }
      const lifeRatio = p.life / p.maxLife;
      p.x += p.vx;
      p.y += p.vy;
      if (p.type !== 'smoke') {
        p.vy -= 0.00005;
      }
      const px = p.x * cssCanvasWidth;
      const py = (1 - p.y) * cssCanvasHeight;
      const alpha = Math.min(1, lifeRatio);
      const currentSize = p.size * (p.type === 'smoke' ? 1 + (1 - lifeRatio) * 1.5 : 1);

      ctx.beginPath();
      ctx.arc(px, py, currentSize, 0, Math.PI * 2);
      if (p.type === 'spark') {
        ctx.fillStyle = `${p.color} ${alpha})`;
        ctx.shadowBlur = 6;
        ctx.shadowColor = `${p.color} ${alpha * 0.8})`;
      } else if (p.type === 'ember') {
        ctx.fillStyle = `${p.color} ${alpha * 0.9})`;
        ctx.shadowBlur = 4;
        ctx.shadowColor = `${p.color} ${alpha * 0.6})`;
      } else {
        ctx.fillStyle = `rgba(80, 80, 90, ${alpha * 0.35})`;
        ctx.shadowBlur = 0;
      }
      ctx.fill();
    }
    ctx.restore();
  }
  ctx.globalAlpha = 1;
}

registerResetFn(resetRendererState);
