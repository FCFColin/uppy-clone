import { $canvas, getCtx, setOnResize } from './renderer_canvas.js';

interface ImageEntry {
  img: HTMLImageElement;
  loaded: boolean;
  url: string;
  fallback: string;
}

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

type StaticCacheInvalidateFn = () => void;
let staticCacheInvalidateFn: StaticCacheInvalidateFn | null = null;

export function registerStaticCacheInvalidate(fn: StaticCacheInvalidateFn): void {
  staticCacheInvalidateFn = fn;
}

setOnResize(refreshBackgroundGradient);

function loadImageEntry(entry: ImageEntry, cacheKey: string): void {
  entry.img.onload = () => {
    entry.loaded = true;
    if ((cacheKey === 'sky' || cacheKey === 'mountains') && staticCacheInvalidateFn) {
      staticCacheInvalidateFn();
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

/** Normalized vertical band where clouds may appear (above mountains). */
export const CLOUD_Y_MIN = 0.06;
export const CLOUD_Y_MAX = 0.52;

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

export const bgState: BackgroundState = {
  stars: [],
  clouds: [],
  gradient: null,
  mountains: [],
  particles: [],
};

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

const CLOUD_LAYERS: CloudLayerConfig[] = [
  { count: 5, yMin: 0.08, yRange: 0.14, widthMin: 0.14, widthRange: 0.08, speedMin: 0.00008, speedRange: 0.00004, opacityMin: 0.28, opacityRange: 0.1, layer: 0 },
  { count: 4, yMin: 0.18, yRange: 0.16, widthMin: 0.11, widthRange: 0.07, speedMin: 0.00012, speedRange: 0.00006, opacityMin: 0.38, opacityRange: 0.1, layer: 1 },
  { count: 3, yMin: 0.28, yRange: 0.14, widthMin: 0.08, widthRange: 0.05, speedMin: 0.00018, speedRange: 0.00008, opacityMin: 0.48, opacityRange: 0.1, layer: 2 },
];

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
    x: Math.random(),
    y: Math.random() * 0.55,
    size: Math.random() * 2 + 1.2,
    twinkle: Math.random() * Math.PI * 2,
  }));

  bgState.clouds = CLOUD_LAYERS.flatMap(cfg =>
    Array.from({ length: cfg.count }, () => spawnCloud(cfg)),
  );

  bgState.mountains = Array.from({ length: 5 }, (_, i) => ({
    x: i * 0.25 - 0.05,
    height: 0.15 + Math.random() * 0.1,
    width: 0.3,
  }));

  bgState.particles = Array.from({ length: 20 }, () => ({
    x: Math.random(), y: Math.random() * 0.8,
    size: 0.5 + Math.random() * 1,
    life: Math.random(),
  }));

  bgState.gradient = getCtx().createLinearGradient(0, 0, 0, $canvas.height);
  bgState.gradient.addColorStop(0, '#0f1729');
  bgState.gradient.addColorStop(0.5, '#16213e');
  bgState.gradient.addColorStop(1, '#1a1a2e');
}

/** Rebuild sky gradient after canvas resize without resetting parallax elements. */
export function refreshBackgroundGradient(): void {
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
