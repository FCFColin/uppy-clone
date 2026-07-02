import { playReadySound, vibrate } from '../shared/ui/audio.js';
import { state } from './state.js';
import { calculateCooldown } from './message_codec.js';
import { $cooldownBar, $cooldownText } from './ui_elements.js';

let cooldownTimer: ReturnType<typeof setInterval> | null = null;

export function startCooldownUpdater(): void {
  stopCooldownUpdater();
  cooldownTimer = setInterval(updateCooldownBar, 100);
}

export function stopCooldownUpdater(): void {
  if (cooldownTimer !== null) {
    clearInterval(cooldownTimer);
    cooldownTimer = null;
  }
  wasReady = false;
}

let wasReady = false;

export function updateCooldownBar(): void {
  if (state.phase !== 'playing') {
    wasReady = false;
    return;
  }
  const now = Date.now();
  if (now < state.myCooldownEnd) {
    wasReady = false;
    const remaining = state.myCooldownEnd - now;
    const total = calculateCooldown(state.players.length);
    const pct = Math.min(100, (remaining / total) * 100);
    $cooldownBar.style.width = pct + '%';
    $cooldownBar.classList.remove('ready');
    $cooldownText.textContent = (remaining / 1000).toFixed(1) + 's';
  } else {
    $cooldownBar.style.width = '0%';
    $cooldownBar.classList.add('ready');
    $cooldownText.textContent = '点击！';
    if (!wasReady) {
      wasReady = true;
      playReadySound();
      vibrate(30);
    }
  }
}
