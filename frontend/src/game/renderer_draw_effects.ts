import { PALETTE_COLORS } from './constants.js';
import { $canvas, getCtx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { state } from './state.js';

const RIPPLE_DURATION_S = 0.6;

function hexToRgb(hex: string): string {
  const h = hex.replace('#', '');
  const n = parseInt(h, 16);
  return `${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}`;
}

function rippleColor(ripple: { playerIndex: number; rejected?: boolean; isOptimistic?: boolean }): string {
  if (ripple.rejected) return '233, 69, 96';
  if (ripple.isOptimistic) return '0, 180, 216';
  const player = state.players.find(p => p.playerIndex === ripple.playerIndex);
  const hex = player
    ? PALETTE_COLORS[player.palette % PALETTE_COLORS.length]!
    : PALETTE_COLORS[ripple.playerIndex % PALETTE_COLORS.length]!;
  return hexToRgb(hex);
}

export function drawRipples(): void {
  const now = Date.now();
  for (let i = state.ripples.length - 1; i >= 0; i--) {
    const ripple = state.ripples[i]!;
    if (now - ripple.time > RIPPLE_DURATION_S * 1000) {
      state.ripples.splice(i, 1);
    }
  }

  for (const ripple of state.ripples) {
    const age = (now - ripple.time) / 1000;
    const t = Math.min(1, age / RIPPLE_DURATION_S);
    if (t >= 1) continue;

    const rx = ripple.x * $canvas.width;
    const ry = (1 - ripple.y) * $canvas.height;
    const maxRadius = Math.min($canvas.width, $canvas.height) * 0.06;
    const radius = maxRadius * (0.3 + 0.7 * t);
    const alpha = (1 - t) * 0.85;

    if (ripple.rejected) {
      getCtx().strokeStyle = `rgba(233, 69, 96, ${alpha})`;
      getCtx().lineWidth = 3;
      const s = 12 + 8 * t;
      getCtx().beginPath();
      getCtx().moveTo(rx - s, ry - s);
      getCtx().lineTo(rx + s, ry + s);
      getCtx().moveTo(rx + s, ry - s);
      getCtx().lineTo(rx - s, ry + s);
      getCtx().stroke();
    } else {
      const rgb = rippleColor(ripple);
      getCtx().beginPath();
      getCtx().arc(rx, ry, radius, 0, Math.PI * 2);
      getCtx().strokeStyle = `rgba(${rgb}, ${alpha})`;
      getCtx().lineWidth = 2;
      getCtx().stroke();
    }
  }
}

export function drawExplosion(): void {
  if (!state.explosionEffect) return;
  const elapsed = Date.now() - state.explosionEffect.startTime;
  const duration = 500;
  if (elapsed > duration) {
    state.explosionEffect = null;
    return;
  }
  if (!gameImages['explosion']!.loaded) return;

  const progress = elapsed / duration;
  const ex = state.explosionEffect.x * $canvas.width;
  const ey = (1 - state.explosionEffect.y) * $canvas.height;
  const baseSize = Math.min($canvas.width, $canvas.height) * 0.15;
  const size = baseSize * (0.5 + progress * 0.5);
  getCtx().globalAlpha = 1 - progress;
  getCtx().drawImage(gameImages['explosion']!.img, ex - size / 2, ey - size / 2, size, size);
  getCtx().globalAlpha = 1;
}
