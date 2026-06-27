import { PALETTE_COLORS } from './constants.js';
import { $canvas, ctx } from './renderer_canvas.js';
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
  const now: number = Date.now();
  for (let i = state.ripples.length - 1; i >= 0; i--) {
    const ripple = state.ripples[i]!;
    if (now - ripple.time > RIPPLE_DURATION_S * 1000) {
      state.ripples.splice(i, 1);
    }
  }

  for (const ripple of state.ripples) {
    const age: number = (now - ripple.time) / 1000;
    const t: number = Math.min(1, age / RIPPLE_DURATION_S);
    if (t >= 1) continue;

    const rx: number = ripple.x * $canvas.width;
    const ry: number = (1 - ripple.y) * $canvas.height;
    const maxRadius: number = Math.min($canvas.width, $canvas.height) * 0.06;
    const radius: number = maxRadius * (0.3 + 0.7 * t);
    const alpha: number = (1 - t) * 0.85;

    if (ripple.rejected) {
      ctx.strokeStyle = `rgba(233, 69, 96, ${alpha})`;
      ctx.lineWidth = 3;
      const s: number = 12 + 8 * t;
      ctx.beginPath();
      ctx.moveTo(rx - s, ry - s);
      ctx.lineTo(rx + s, ry + s);
      ctx.moveTo(rx + s, ry - s);
      ctx.lineTo(rx - s, ry + s);
      ctx.stroke();
    } else {
      const rgb = rippleColor(ripple);
      ctx.beginPath();
      ctx.arc(rx, ry, radius, 0, Math.PI * 2);
      ctx.strokeStyle = `rgba(${rgb}, ${alpha})`;
      ctx.lineWidth = 2;
      ctx.stroke();
    }
  }
}

export function drawExplosion(): void {
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
