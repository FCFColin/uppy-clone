import { dispatch, getState, type ClientPlayer } from './state.js';
import {
  commitRenderedState,
  getInterpolatedBalloon,
  getInterpolatedBird,
  getInterpolatedGhost,
} from './state_interp.js';
import {
  drawTutorialRangeCircle,
  drawDangerVignettes,
  drawFloatingTexts,
  fillCircle,
  drawImageAlpha,
  drawRadialGlow,
  isCollisionDebugEnabled,
  drawCollisionDebug,
} from './visual_helpers.js';
import { drainPendingMessages } from './ws_connection.js';
import { PALETTE_COLORS } from '../shared/game/constants.js';
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
  brightness: number;
  twinkleSpeed: number;
  twinklePhase: number;
  twinkleOffset: number;
  colorTemp: 'warm' | 'cool' | 'neutral';
}

export interface Meteor {
  x: number;
  y: number;
  vx: number;
  vy: number;
  life: number;
  maxLife: number;
  length: number;
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

export interface MountainLayer {
  mountains: Mountain[];
  color: string;
  baseY: number;
  snowCaps?: boolean;
}

export interface Particle {
  x: number;
  y: number;
  size: number;
  life: number;
}

export interface TrailParticle {
  x: number;
  y: number;
  life: number;
  size: number;
}

export interface BackgroundState {
  stars: Star[];
  meteors: Meteor[];
  clouds: Cloud[];
  gradient: CanvasGradient | null;
  mountainLayers: MountainLayer[];
  forestTrees: { x: number; height: number; width: number }[];
  particles: Particle[];
  lastMeteorTime: number;
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

export const gameImages: Record<string, ImageEntry> = {
  sky: { img: new Image(), loaded: false, url: '/assets/sky.jpg', fallback: '/assets/fallback/sky-bg.svg' },
  cloud: { img: new Image(), loaded: false, url: '/assets/cloud1.jpg', fallback: '/assets/fallback/cloud-1.svg' },
  mountains: {
    img: new Image(),
    loaded: false,
    url: '/assets/mountains.jpg',
    fallback: '/assets/fallback/mountains.svg',
  },
  ghost: {
    img: new Image(),
    loaded: false,
    url: '/assets/ghost.jpg',
    fallback: '/assets/fallback/enemy-ghost.svg',
  },
  balloon: {
    img: new Image(),
    loaded: false,
    url: '/assets/balloon.jpg',
    fallback: '/assets/fallback/balloon-red.svg',
  },
  explosion: {
    img: new Image(),
    loaded: false,
    url: '/assets/explosion.jpg',
    fallback: '/assets/fallback/explosion.svg',
  },
};

const cloudImages: ImageEntry[] = [
  { img: new Image(), loaded: false, url: '/assets/cloud1.jpg', fallback: '/assets/fallback/cloud-1.svg' },
  { img: new Image(), loaded: false, url: '/assets/cloud2.jpg', fallback: '/assets/fallback/cloud-2.svg' },
  { img: new Image(), loaded: false, url: '/assets/cloud3.jpg', fallback: '/assets/fallback/cloud-3.svg' },
];

const bgState: BackgroundState = {
  stars: [],
  meteors: [],
  clouds: [],
  gradient: null,
  mountainLayers: [],
  forestTrees: [],
  particles: [],
  lastMeteorTime: 0,
};

const CLOUD_LAYERS: CloudLayerConfig[] = [
  {
    count: 5,
    yMin: 0.08,
    yRange: 0.14,
    widthMin: 0.08,
    widthRange: 0.05,
    speedMin: 0.00008,
    speedRange: 0.00004,
    opacityMin: 0.2,
    opacityRange: 0.1,
    layer: 0,
  },
  {
    count: 4,
    yMin: 0.18,
    yRange: 0.16,
    widthMin: 0.11,
    widthRange: 0.07,
    speedMin: 0.00012,
    speedRange: 0.00006,
    opacityMin: 0.35,
    opacityRange: 0.12,
    layer: 1,
  },
  {
    count: 3,
    yMin: 0.28,
    yRange: 0.14,
    widthMin: 0.15,
    widthRange: 0.08,
    speedMin: 0.00018,
    speedRange: 0.00008,
    opacityMin: 0.5,
    opacityRange: 0.15,
    layer: 2,
  },
];

let layout: CanvasLayout = { top: 0, bottom: 0, width: 0, height: 0 };
let fallbackCanvas: HTMLCanvasElement | null = null;
let _ctx: CanvasRenderingContext2D | null = null;
let staticCanvas: HTMLCanvasElement | null = null;
let staticCtx: CanvasRenderingContext2D | null = null;
let staticCacheW = 0;
let staticCacheH = 0;
let moonCanvas: HTMLCanvasElement | null = null;
let moonCtx: CanvasRenderingContext2D | null = null;
let moonCacheW = 0;
let moonCacheH = 0;
let renderActive = true;
let loopRunning = false;
let cachedPlayerMap: Map<number, ClientPlayer> | null = null;
let cachedPlayerMapKey: string | null = null;
let _previousTimestamp: number | undefined;
let trailParticles: TrailParticle[] = [];
let trailFrameCounter = 0;

function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function measureLayoutInsets(): CanvasLayout {
  const hud = document.getElementById('game-hud');
  const cooldown = document.getElementById('cooldown-indicator');
  let top = 0;
  let bottom = 0;
  if (hud && !hud.classList.contains('hidden')) {
    const hudInner = hud.querySelector('.hud-inner') as HTMLElement | null;
    top = hudInner ? hudInner.offsetHeight : 0;
    const hudBottomBar = hud.querySelector('.hud-bottom-bar') as HTMLElement | null;
    if (hudBottomBar) {
      bottom = Math.max(bottom, hudBottomBar.offsetHeight);
    }
  }
  if (cooldown && !cooldown.classList.contains('hidden')) {
    bottom = Math.max(bottom, cooldown.offsetHeight + 12);
  }
  layout = { top, bottom, width: window.innerWidth, height: Math.max(1, window.innerHeight - top - bottom) };
  return layout;
}

export const $canvas = (document.getElementById('game-canvas') ??
  (() => {
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

function initImages(): void {
  for (const key in gameImages) {
    loadImageEntry(gameImages[key]!, key);
  }
  for (const entry of cloudImages) {
    loadImageEntry(entry, entry.url);
  }
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

function createDeterministicMountains(
  positions: number[],
  heightMin: number,
  heightMax: number,
  widthBase: number,
  seed: number,
): Mountain[] {
  const seededRandom = (i: number): number => {
    const x = Math.sin(seed + i * 127.1) * 43758.5453;
    return x - Math.floor(x);
  };
  return positions.map((pos, i) => ({
    x: pos,
    height: heightMin + seededRandom(i * 3) * (heightMax - heightMin),
    width: widthBase + seededRandom(i * 3 + 1) * 0.08,
  }));
}

function initBackground(): void {
  initImages();
  bgState.stars = Array.from({ length: 260 }, (_, i) => {
    const seeded = (n: number): number => {
      const x = Math.sin(n * 9301 + 49297 + i * 127.1) * 233280;
      return x - Math.floor(x);
    };
    const sizeCategory = seeded(i * 5);
    let size: number;
    let brightness: number;
    let twinkleSpeed: number;
    let colorTemp: Star['colorTemp'];
    if (sizeCategory < 0.6) {
      size = 0.5 + seeded(i * 5 + 1) * 0.8;
      brightness = 0.3 + seeded(i * 5 + 2) * 0.4;
      twinkleSpeed = 0.8 + seeded(i * 5 + 3) * 1.2;
    } else if (sizeCategory < 0.9) {
      size = 1.3 + seeded(i * 5 + 1) * 1.2;
      brightness = 0.6 + seeded(i * 5 + 2) * 0.3;
      twinkleSpeed = 0.5 + seeded(i * 5 + 3) * 0.8;
    } else {
      size = 2.5 + seeded(i * 5 + 1) * 1.5;
      brightness = 0.8 + seeded(i * 5 + 2) * 0.2;
      twinkleSpeed = 0.3 + seeded(i * 5 + 3) * 0.5;
    }
    const tempR = seeded(i * 5 + 4);
    colorTemp = tempR < 0.15 ? 'warm' : tempR < 0.8 ? 'neutral' : 'cool';
    return {
      x: seeded(i * 4),
      y: seeded(i * 4 + 1) * 0.82,
      size,
      brightness,
      twinkleSpeed,
      twinklePhase: seeded(i * 4 + 2) * Math.PI * 2,
      twinkleOffset: seeded(i * 4 + 3) * 0.3,
      colorTemp,
    };
  });
  bgState.meteors = [];
  bgState.lastMeteorTime = 0;
  bgState.clouds = CLOUD_LAYERS.flatMap((cfg, li) =>
    Array.from({ length: cfg.count }, (_, ci) => {
      const seedBase = li * 100 + ci;
      const s = (n: number): number => {
        const x = Math.sin(seedBase + n * 127.1) * 43758.5453;
        return x - Math.floor(x);
      };
      return {
        x: s(0),
        y: cfg.yMin + s(1) * cfg.yRange,
        width: cfg.widthMin + s(2) * cfg.widthRange,
        speed: cfg.speedMin + s(3) * cfg.speedRange,
        opacity: cfg.opacityMin + s(4) * cfg.opacityRange,
        layer: cfg.layer,
        variant: Math.floor(s(5) * cloudImages.length),
        bobPhase: s(6) * Math.PI * 2,
      };
    }),
  );

  bgState.mountainLayers = [
    {
      mountains: createDeterministicMountains(
        [0.03, 0.15, 0.28, 0.42, 0.55, 0.68, 0.82, 0.95],
        0.06,
        0.12,
        0.20,
        1001,
      ),
      color: 'rgba(30, 28, 75, 0.35)',
      baseY: 0.78,
    },
    {
      mountains: createDeterministicMountains(
        [0.08, 0.22, 0.38, 0.52, 0.68, 0.85],
        0.10,
        0.16,
        0.25,
        2002,
      ),
      color: 'rgba(22, 22, 62, 0.55)',
      baseY: 0.83,
    },
    {
      mountains: createDeterministicMountains(
        [0.12, 0.32, 0.50,