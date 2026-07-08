import { PALETTE_COLORS } from '../shared/game/constants.js';
import { $canvas, getCtx } from './renderer_canvas.js';
import { gameImages } from './renderer_background.js';
import { dispatch, getState } from './store.js';

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

let _pruneScheduled = false;

export function pruneEffects(): void {
  if (_pruneScheduled) return;
  _pruneScheduled = true;
  requestAnimationFrame(() => {
    _pruneScheduled = false;
    const currentRipples = getState().ripples;
    const remaining = currentRipples.filter(r => Date.now() - r.time <= RIPPLE_DURATION_S * 1000).slice(-50);
    if (remaining.length !== currentRipples.length) {
      dispatch({ type: 'SET_STATE', partial: { ripples: remaining } });
    }

    const explosion = getState().explosionEffect;
    if (explosion && Date.now() - explosion.startTime > 500) {
      dispatch({ type: 'SET_STATE', partial: { explosionEffect: null } });
    }
  });
}

export function drawRipples(now: number, playerMap: Map<number, { palette: number }>): void {
  const remaining = getState().ripples;

  const ctx = getCtx();
  for (const ripple of remaining) {
    const age = (now - ripple.time) / 1000;
    const t = Math.min(1, age / RIPPLE_DURATION_S);
    if (t >= 1) continue;

    const rx = ripple.x * $canvas.width;
    const ry = (1 - ripple.y) * $canvas.height;
    const maxRadius = Math.min($canvas.width, $canvas.height) * 0.06;
    const radius = maxRadius * (0.3 + 0.7 * t);
    const alpha = (1 - t) * 0.85;

    if (ripple.rejected) {
      ctx.globalAlpha = alpha;
      ctx.strokeStyle = _rejectedRgb + ')';
      ctx.lineWidth = 3;
      const s = 12 + 8 * t;
      ctx.beginPath();
      ctx.moveTo(rx - s, ry - s);
      ctx.lineTo(rx + s, ry + s);
      ctx.moveTo(rx + s, ry - s);
      ctx.lineTo(rx - s, ry + s);
      ctx.stroke();
    } else {
      const { base } = rippleColor(ripple, playerMap);
      ctx.beginPath();
      ctx.arc(rx, ry, radius, 0, Math.PI * 2);
      ctx.globalAlpha = alpha;
      ctx.strokeStyle = base + ')';
      ctx.lineWidth = 2;
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  }
}

export function drawExplosion(now: number): void {
  const explosion = getState().explosionEffect;
  if (!explosion) return;
  if (!gameImages['explosion']!.loaded) return;

  const elapsed = now - explosion.startTime;
  const duration = 500;
  const ctx = getCtx();
  const progress = elapsed / duration;
  const ex = explosion.x * $canvas.width;
  const ey = (1 - explosion.y) * $canvas.height;
  const baseSize = Math.min($canvas.width, $canvas.height) * 0.15;
  const size = baseSize * (0.5 + progress * 0.5);
  ctx.globalAlpha = 1 - progress;
  ctx.drawImage(gameImages['explosion']!.img, ex - size / 2, ey - size / 2, size, size);
  ctx.globalAlpha = 1;
}
