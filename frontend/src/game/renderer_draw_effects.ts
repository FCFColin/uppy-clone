import { PALETTE_COLORS } from './local_constants.js';
import { $canvas, getCtx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { state } from './state_types.js';

const RIPPLE_DURATION_S = 0.6;

const _rejectedRgb = 'rgba(233,69,96,';
const _optimisticRgb = 'rgba(0,180,216,';

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

export function drawRipples(now: number, playerMap: Map<number, { palette: number }>): void {
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
      getCtx().globalAlpha = alpha;
      getCtx().strokeStyle = _rejectedRgb + ')';
      getCtx().lineWidth = 3;
      const s = 12 + 8 * t;
      getCtx().beginPath();
      getCtx().moveTo(rx - s, ry - s);
      getCtx().lineTo(rx + s, ry + s);
      getCtx().moveTo(rx + s, ry - s);
      getCtx().lineTo(rx - s, ry + s);
      getCtx().stroke();
      getCtx().globalAlpha = 1;
    } else {
      const { base } = rippleColor(ripple, playerMap);
      getCtx().beginPath();
      getCtx().arc(rx, ry, radius, 0, Math.PI * 2);
      getCtx().globalAlpha = alpha;
      getCtx().strokeStyle = base + ')';
      getCtx().lineWidth = 2;
      getCtx().stroke();
      getCtx().globalAlpha = 1;
    }
  }
}

export function drawExplosion(now: number): void {
  if (!state.explosionEffect) return;
  const elapsed = now - state.explosionEffect.startTime;
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
