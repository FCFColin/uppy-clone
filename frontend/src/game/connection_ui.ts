import { routeConnectionError, type EntryFullScreenErrorOptions } from './entry_flow.js';

export type ConnectionErrorOptions = EntryFullScreenErrorOptions;

export function showConnectionError(message: string, options?: ConnectionErrorOptions): void {
  routeConnectionError(message, options);
}

let retryBound = false;

export function hideReconnectBanner(): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  if ($banner) $banner.classList.add('hidden');
}

export function showReconnectBanner(attempt: number): void {
  const $banner: HTMLElement | null = document.getElementById('reconnect-banner');
  const $text: HTMLElement | null = document.getElementById('reconnect-text');
  if ($text) $text.textContent = `网络断开，正在重连…（第${attempt}次尝试）`;
  if ($banner) $banner.classList.remove('hidden');
}

export function updatePingDisplay(rttMs: number): void {
  if (!Number.isFinite(rttMs)) return;
  const $ping: HTMLElement | null = document.getElementById('ping-display');
  if (!$ping) return;
  $ping.classList.toggle('hidden', rttMs <= 150);
  if (rttMs > 150) {
    $ping.textContent = `${rttMs}ms`;
    $ping.classList.toggle('ping-unstable', rttMs > 200);
  }
}

export function bindReconnectRetry(retryFn: () => void): void {
  if (retryBound) return;
  retryBound = true;
  const btn = document.getElementById('reconnect-retry-btn');
  btn?.addEventListener('click', () => retryFn());
}


