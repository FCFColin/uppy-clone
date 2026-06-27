import { refreshBackgroundGradient } from './renderer_background_data.js';

/** Canvas layout: reserves space for HUD and cooldown bar (portrait-friendly). */
export interface CanvasLayout {
  top: number;
  bottom: number;
  width: number;
  height: number;
}

let layout: CanvasLayout = { top: 0, bottom: 0, width: 0, height: 0 };
let fallbackCanvas: HTMLCanvasElement | null = null;

export function getCanvasLayout(): CanvasLayout {
  return layout;
}

export function measureLayoutInsets(): CanvasLayout {
  const hud = document.getElementById('game-hud');
  const cooldown = document.getElementById('cooldown-indicator');
  let top = 0;
  let bottom = 0;

  if (hud && !hud.classList.contains('hidden')) {
    top = hud.offsetHeight;
  }
  if (cooldown && !cooldown.classList.contains('hidden')) {
    bottom = cooldown.offsetHeight + 12;
  }

  layout = {
    top,
    bottom,
    width: window.innerWidth,
    height: Math.max(1, window.innerHeight - top - bottom),
  };
  return layout;
}

export const $canvas: HTMLCanvasElement = (document.getElementById('game-canvas') ?? (() => {
  fallbackCanvas = document.createElement('canvas');
  return fallbackCanvas;
})()) as HTMLCanvasElement;

function resolveCtx(): CanvasRenderingContext2D {
  const c = $canvas.getContext('2d');
  if (!c) throw new Error('game canvas 2d context unavailable');
  return c;
}

export const ctx: CanvasRenderingContext2D = new Proxy({} as CanvasRenderingContext2D, {
  get(_target, prop, receiver) {
    return Reflect.get(resolveCtx(), prop, receiver);
  },
});

export function resizeCanvas(): void {
  measureLayoutInsets();
  $canvas.width = layout.width;
  $canvas.height = layout.height;
  if (document.getElementById('game-canvas')) {
    $canvas.style.top = `${layout.top}px`;
    $canvas.style.bottom = `${layout.bottom}px`;
    $canvas.style.height = `${layout.height}px`;
  }
  refreshBackgroundGradient();
}

/** Map client coords to normalized game space accounting for canvas offset. */
export function clientToNormalized(clientX: number, clientY: number): { x: number; y: number } {
  const rect = $canvas.getBoundingClientRect();
  const w = rect.width || 1;
  const h = rect.height || 1;
  return {
    x: (clientX - rect.left) / w,
    y: 1 - (clientY - rect.top) / h,
  };
}
