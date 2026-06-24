import { FIXED_STEP } from './constants.js';
import {
  state,
  getInterpolatedBalloon,
  getInterpolatedGhost,
} from './state.js';
import { updateCooldownBar } from './ui.js';

export const $canvas: HTMLCanvasElement = document.getElementById('game-canvas') as HTMLCanvasElement;
const ctx: CanvasRenderingContext2D = $canvas.getContext('2d')!;

export function resizeCanvas(): void {
  $canvas.width = window.innerWidth;
  $canvas.height = window.innerHeight;
}

interface ImageEntry {
  img: HTMLImageElement;
  loaded: boolean;
  url: string;
  fallback: string;
}

const gameImages: Record<string, ImageEntry> = {
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

interface Star {
  x: number;
  y: number;
  size: number;
  twinkle: number;
}

interface Cloud {
  x: number;
  y: number;
  width: number;
  speed: number;
  opacity: number;
  layer: number;
}

interface Mountain {
  x: number;
  height: number;
  width: number;
}

interface Particle {
  x: number;
  y: number;
  size: number;
  life: number;
}

let bgStars: Star[] = [];
let bgClouds: Cloud[] = [];
let bgGradient: CanvasGradient | null = null;
let bgMountains: Mountain[] = [];
let bgParticles: Particle[] = [];

function initBackground(): void {
  bgStars = [];
  for (let i = 0; i < 50; i++) {
    bgStars.push({
      x: Math.random(),
      y: Math.random() * 0.7,
      size: Math.random() * 1.5 + 0.5,
      twinkle: Math.random() * Math.PI * 2,
    });
  }

  bgClouds = [];
  for (let i = 0; i < 4; i++) {
    bgClouds.push({
      x: Math.random(), y: 0.1 + Math.random() * 0.2,
      width: 0.2 + Math.random() * 0.1,
      speed: 0.000015 + Math.random() * 0.00001,
      opacity: 0.04 + Math.random() * 0.04, layer: 0,
    });
  }
  for (let i = 0; i < 3; i++) {
    bgClouds.push({
      x: Math.random(), y: 0.25 + Math.random() * 0.2,
      width: 0.15 + Math.random() * 0.08,
      speed: 0.00003 + Math.random() * 0.00002,
      opacity: 0.06 + Math.random() * 0.06, layer: 1,
    });
  }
  for (let i = 0; i < 2; i++) {
    bgClouds.push({
      x: Math.random(), y: 0.4 + Math.random() * 0.15,
      width: 0.1 + Math.random() * 0.05,
      speed: 0.00005 + Math.random() * 0.00003,
      opacity: 0.08 + Math.random() * 0.08, layer: 2,
    });
  }

  bgMountains = [];
  for (let i = 0; i < 5; i++) {
    bgMountains.push({
      x: i * 0.25 - 0.05,
      height: 0.08 + Math.random() * 0.06,
      width: 0.3,
    });
  }

  bgParticles = [];
  for (let i = 0; i < 20; i++) {
    bgParticles.push({
      x: Math.random(), y: Math.random() * 0.8,
      size: 0.5 + Math.random() * 1,
      life: Math.random(),
    });
  }

  bgGradient = ctx.createLinearGradient(0, 0, 0, $canvas.height);
  bgGradient.addColorStop(0, '#0f1729');
  bgGradient.addColorStop(0.5, '#16213e');
  bgGradient.addColorStop(1, '#1a1a2e');
}

function drawBackground(): void {
  if (!bgGradient) initBackground();

  if (gameImages['sky']!.loaded) {
    ctx.drawImage(gameImages['sky']!.img, 0, 0, $canvas.width, $canvas.height);
  } else if (bgGradient) {
    ctx.fillStyle = bgGradient;
    ctx.fillRect(0, 0, $canvas.width, $canvas.height);
  }

  const time: number = Date.now() * 0.001;
  for (const star of bgStars) {
    const alpha: number = 0.3 + Math.sin(time + star.twinkle) * 0.3;
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
    for (const m of bgMountains) {
      const mx: number = m.x * $canvas.width;
      const my: number = $canvas.height - m.height * $canvas.height;
      ctx.lineTo(mx, my);
      ctx.lineTo(mx + m.width * $canvas.width * 0.5, $canvas.height);
    }
    ctx.lineTo($canvas.width, $canvas.height);
    ctx.closePath();
    ctx.fill();
  }

  for (const cloud of bgClouds) {
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
  for (const p of bgParticles) {
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

function drawBalloon(): void {
  const interp = getInterpolatedBalloon();
  const bx: number = interp.x * $canvas.width;
  const by: number = (1 - interp.y) * $canvas.height;
  const radius: number = Math.min($canvas.width, $canvas.height) * 0.06;

  if (gameImages['balloon']!.loaded) {
    const img: HTMLImageElement = gameImages['balloon']!.img;
    const w: number = radius * 2.5;
    const h: number = w * (img.height / img.width);
    ctx.drawImage(img, bx - w / 2, by - h / 2, w, h);
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

function drawBird(): void {
  const bx: number = state.bird.x * $canvas.width;
  const by: number = (1 - state.bird.y) * $canvas.height;
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

function drawGhost(): void {
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

function drawRipples(): void {
  const now: number = Date.now();
  for (let i = state.ripples.length - 1; i >= 0; i--) {
    const ripple = state.ripples[i]!;
    if (now - ripple.time > 2000) {
      state.ripples.splice(i, 1);
    }
  }

  for (const ripple of state.ripples) {
    const age: number = (now - ripple.time) / 1000;
    const rx: number = ripple.x * $canvas.width;
    const ry: number = (1 - ripple.y) * $canvas.height;
    const maxRadius: number = Math.min($canvas.width, $canvas.height) * 0.08;
    const radius: number = maxRadius * age;
    const alpha: number = 1 - age;

    if (ripple.rejected) {
      ctx.strokeStyle = `rgba(233, 69, 96, ${alpha})`;
      ctx.lineWidth = 3;
      const s: number = 15;
      ctx.beginPath();
      ctx.moveTo(rx - s, ry - s);
      ctx.lineTo(rx + s, ry + s);
      ctx.moveTo(rx + s, ry - s);
      ctx.lineTo(rx - s, ry + s);
      ctx.stroke();
    } else {
      const color: string = ripple.optimistic ? '0, 180, 216' : '233, 69, 96';
      ctx.beginPath();
      ctx.arc(rx, ry, radius, 0, Math.PI * 2);
      ctx.strokeStyle = `rgba(${color}, ${alpha})`;
      ctx.lineWidth = 2;
      ctx.stroke();
    }
  }
}

function drawExplosion(): void {
  if (!state.explosionEffect) return;
  const elapsed: number = Date.now() - state.explosionEffect.startTime;
  const duration: number = 500;
  if (elapsed > duration) {
    state.explosionEffect = null;
    return;
  }
  if (!gameImages['explosion']!.loaded) return;

  const progress: number = elapsed / duration;
  const ex: number = state.explosionEffect.x * $canvas.width;
  const ey: number = (1 - state.explosionEffect.y) * $canvas.height;
  const baseSize: number = Math.min($canvas.width, $canvas.height) * 0.15;
  const size: number = baseSize * (0.5 + progress * 0.5);
  ctx.globalAlpha = 1 - progress;
  ctx.drawImage(gameImages['explosion']!.img, ex - size / 2, ey - size / 2, size, size);
  ctx.globalAlpha = 1;
}

let renderActive: boolean = true;
let lastFrameTime: number = 0;

export function setRenderActive(active: boolean): void {
  renderActive = active;
}

export function gameLoop(timestamp: number): void {
  if (!renderActive) return;
  const delta: number = timestamp - lastFrameTime;
  if (delta >= FIXED_STEP) {
    render();
    lastFrameTime = timestamp;
  }
  requestAnimationFrame(gameLoop);
}

function render(): void {
  try {
    ctx.clearRect(0, 0, $canvas.width, $canvas.height);
    drawBackground();

    if (state.phase === 'playing' || state.phase === 'ended') {
      if (state.hasReceivedFirstSnapshot) {
        drawBalloon();
        if (state.bird.active) drawBird();
        drawGhost();
      }
      drawRipples();
      drawExplosion();
    }

    if (state.phase === 'playing') {
      updateCooldownBar();
    }
  } catch (err: unknown) {
    console.error('Render error:', err);
  }
}
