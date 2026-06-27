import { refreshBackgroundGradient } from './renderer_background_data.js';

export const $canvas: HTMLCanvasElement = document.getElementById('game-canvas') as HTMLCanvasElement;
export const ctx: CanvasRenderingContext2D = $canvas.getContext('2d')!;

export function resizeCanvas(): void {
  $canvas.width = window.innerWidth;
  $canvas.height = window.innerHeight;
  refreshBackgroundGradient();
}
