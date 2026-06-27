import { $canvas, ctx } from './renderer_canvas.js';

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

for (const key in gameImages) {
  const entry: ImageEntry = gameImages[key]!;
  entry.img.onload = () => { entry.loaded = true; };
  entry.img.onerror = () => {
    entry.img.onerror = null;
    entry.img.src = entry.fallback;
  };
  entry.img.src = entry.url;
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

export function initBackground(): void {
  bgState.stars = [];
  for (let i = 0; i < 50; i++) {
    bgState.stars.push({
      x: Math.random(),
      y: Math.random() * 0.7,
      size: Math.random() * 1.5 + 0.5,
      twinkle: Math.random() * Math.PI * 2,
    });
  }

  bgState.clouds = [];
  for (let i = 0; i < 4; i++) {
    bgState.clouds.push({
      x: Math.random(), y: 0.1 + Math.random() * 0.2,
      width: 0.2 + Math.random() * 0.1,
      speed: 0.000015 + Math.random() * 0.00001,
      opacity: 0.04 + Math.random() * 0.04, layer: 0,
    });
  }
  for (let i = 0; i < 3; i++) {
    bgState.clouds.push({
      x: Math.random(), y: 0.25 + Math.random() * 0.2,
      width: 0.15 + Math.random() * 0.08,
      speed: 0.00003 + Math.random() * 0.00002,
      opacity: 0.06 + Math.random() * 0.06, layer: 1,
    });
  }
  for (let i = 0; i < 2; i++) {
    bgState.clouds.push({
      x: Math.random(), y: 0.4 + Math.random() * 0.15,
      width: 0.1 + Math.random() * 0.05,
      speed: 0.00005 + Math.random() * 0.00003,
      opacity: 0.08 + Math.random() * 0.08, layer: 2,
    });
  }

  bgState.mountains = [];
  for (let i = 0; i < 5; i++) {
    bgState.mountains.push({
      x: i * 0.25 - 0.05,
      height: 0.08 + Math.random() * 0.06,
      width: 0.3,
    });
  }

  bgState.particles = [];
  for (let i = 0; i < 20; i++) {
    bgState.particles.push({
      x: Math.random(), y: Math.random() * 0.8,
      size: 0.5 + Math.random() * 1,
      life: Math.random(),
    });
  }

  bgState.gradient = ctx.createLinearGradient(0, 0, 0, $canvas.height);
  bgState.gradient.addColorStop(0, '#0f1729');
  bgState.gradient.addColorStop(0.5, '#16213e');
  bgState.gradient.addColorStop(1, '#1a1a2e');
}

/** Rebuild sky gradient after canvas resize without resetting parallax elements. */
export function refreshBackgroundGradient(): void {
  bgState.gradient = ctx.createLinearGradient(0, 0, 0, $canvas.height);
  bgState.gradient.addColorStop(0, '#0f1729');
  bgState.gradient.addColorStop(0.5, '#16213e');
  bgState.gradient.addColorStop(1, '#1a1a2e');
}
