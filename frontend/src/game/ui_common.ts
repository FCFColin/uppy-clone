import { dispatch, getState } from './state.js';
import { pickRandomNickname, $cooldownBar, $cooldownText } from './ui_elements.js';
import { showToast } from '../shared/ui/utils.js';
import { playCountdownTick, playReadySound, vibrate } from '../shared/ui/audio.js';
import { resizeCanvas } from './renderer.js';
import { calculateCooldown } from './message_codec.js';
import { isTutorialDone } from '../shared/data/cookies.js';
import { PHYSICS } from '../shared/game/constants.js';
import { routeConnectionError, type EntryFullScreenErrorOptions } from './entry_flow.js';

// ===== Countdown (from ui_utils) =====

export function startCountdownTimer(seconds: number): void {
  if (seconds <= 0) {
    hideCountdownOverlay();
    return;
  }
  const existing = getState().countdownTimerInterval;
  if (existing !== null) {
    clearInterval(existing);
    dispatch({ type: 'SET_STATE', partial: { countdownTimerInterval: null } });
  }
  const countdownEl: HTMLElement | null = document.getElementById('countdown-overlay');
  if (!countdownEl) return;
  const numberEl: Element | null = countdownEl.querySelector('.countdown-number');
  let remaining: number = seconds;
  if (numberEl) {
    numberEl.textContent = String(remaining);
    numberEl.classList.remove('countdown-pop');
    void numberEl.clientWidth;
    numberEl.classList.add('countdown-pop');
  }
  playCountdownTick();
  countdownEl.classList.remove('hidden');

  const timer = setInterval(() => {
    remaining--;
    if (remaining <= 0) {
      clearInterval(timer);
      dispatch({ type: 'SET_STATE', partial: { countdownTimerInterval: null } });
      countdownEl.classList.add('hidden');
    } else {
      if (numberEl) {
        numberEl.textContent = String(remaining);
        numberEl.classList.remove('countdown-pop');
        void numberEl.clientWidth;
        numberEl.classList.add('countdown-pop');
      }
      playCountdownTick();
    }
  }, 1000);
  dispatch({ type: 'SET_STATE', partial: { countdownTimerInterval: timer } });
}

export function hideCountdownOverlay(): void {
  document.getElementById('countdown-overlay')?.classList.add('hidden');
}

export function showCountdownOverlay(): void {
  document.getElementById('countdown-overlay')?.classList.remove('hidden');
}


// ===== Nickname / copy / layout / fallback (from ui_utils) =====

export function generateRandomNickname(): string {
  return pickRandomNickname();
}

export async function copyCode(): Promise<void> {
  const url: string = `${window.location.origin}/play.html?code=${getState().lobbyCode}`;
  try {
    await navigator.clipboard.writeText(url);
    showToast('已复制邀请链接');
  } catch {
    showToast('复制失败，请手动复制房间码');
  }
}

export function refreshLayout(): void {
  resizeCanvas();
}

export function showFallbackErrorScreen(message: string): void {
  if (document.getElementById('game-fallback-error')) return;
  const overlay: HTMLDivElement = document.createElement('div');
  overlay.id = 'game-fallback-error';
  overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.8);z-index:99999;display:flex;align-items:center;justify-content:center;flex-direction:column;color:#fff;font-family:sans-serif;';

  const h2: HTMLHeadingElement = document.createElement('h2');
  h2.style.marginBottom = '1rem';
  h2.textContent = '\u{1F635} 出错了';

  const p: HTMLParagraphElement = document.createElement('p');
  p.style.cssText = 'margin-bottom:1.5rem;color:#ccc;';
  p.textContent = message;

  const btn: HTMLButtonElement = document.createElement('button');
  btn.style.cssText = 'padding:0.8rem 2rem;font-size:1rem;cursor:pointer;border:none;border-radius:8px;background:#0f3460;color:#fff;';
  btn.textContent = '刷新页面';
  btn.onclick = () => location.reload();

  overlay.appendChild(h2);
  overlay.appendChild(p);
  overlay.appendChild(btn);
  document.body.appendChild(overlay);
}


// ===== Cooldown bar (from ui_cooldown) =====

let cooldownTimer: ReturnType<typeof setInterval> | null = null;
let wasReady = false;

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

export function updateCooldownBar(): void {
  if (getState().phase !== 'playing') {
    wasReady = false;
    return;
  }
  const now = Date.now();
  if (now < getState().myCooldownEnd) {
    wasReady = false;
    const remaining = getState().myCooldownEnd - now;
    const total = calculateCooldown(getState().players.length);
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


// ===== Wind indicator (from ui_wind) =====

const WIND_CLAMP = PHYSICS.WIND_CLAMP;
const STRONG_WIND_THRESHOLD = 0.55;

let $windIndicator: HTMLElement | null = null;
let $windDirection: HTMLElement | null = null;
let $windMeterFill: HTMLElement | null = null;
let $windStrength: HTMLElement | null = null;
let $windFirstHint: HTMLElement | null = null;
let windHintShown = false;

function ensureWindElements(): void {
  if ($windIndicator && !$windIndicator.isConnected) {
    $windIndicator = null;
    $windDirection = null;
    $windMeterFill = null;
    $windStrength = null;
    $windFirstHint = null;
  }
  if (!$windIndicator) $windIndicator = document.getElementById('wind-indicator');
  if (!$windDirection) $windDirection = document.getElementById('wind-direction');
  if (!$windMeterFill) $windMeterFill = document.getElementById('wind-meter-fill');
  if (!$windStrength) $windStrength = document.getElementById('wind-strength');
  if (!$windFirstHint) $windFirstHint = document.getElementById('wind-first-hint');
}

export function updateWindIndicator(wind: number): void {
  ensureWindElements();
  if (!$windIndicator || !$windDirection || !$windMeterFill || !$windStrength) return;

  const visible = getState().phase === 'playing';
  $windIndicator.classList.toggle('hidden', !visible);
  if (!visible) return;

  const clamped = Math.max(-WIND_CLAMP, Math.min(WIND_CLAMP, wind));
  const magnitude = Math.abs(clamped) / WIND_CLAMP;
  const pct = Math.round(magnitude * 100);
  const isCalm = magnitude < 0.08;

  $windDirection.textContent = windDirArrow(clamped, isCalm);
  $windDirection.style.color = magnitude >= STRONG_WIND_THRESHOLD ? '#ffb4c4' : '#a8d4ff';
  $windStrength.textContent = `${pct}%`;

  $windMeterFill.classList.toggle('strong', magnitude >= STRONG_WIND_THRESHOLD);
  applyWindMeterStyle($windMeterFill, clamped, magnitude, isCalm);

  const dirLabel = windDirLabel(clamped, isCalm);
  $windIndicator.title = `风向 ${dirLabel} · 风力 ${pct}%`;

  if (!windHintShown && !isTutorialDone() && $windFirstHint) {
    windHintShown = true;
    $windFirstHint.classList.remove('hidden');
    setTimeout(() => $windFirstHint?.classList.add('hidden'), 3000);
  }
}

function windDirArrow(clamped: number, isCalm: boolean): string {
  if (isCalm) return '·';
  return clamped >= 0 ? '→' : '←';
}

function windDirLabel(clamped: number, isCalm: boolean): string {
  if (isCalm) return '无';
  return clamped >= 0 ? '东' : '西';
}

function applyWindMeterStyle(el: HTMLElement, clamped: number, magnitude: number, isCalm: boolean): void {
  if (isCalm) {
    el.style.width = '0%';
    el.style.left = '50%';
    el.style.right = 'auto';
  } else if (clamped >= 0) {
    el.style.left = '50%';
    el.style.right = 'auto';
    el.style.width = `${(magnitude * 50).toFixed(1)}%`;
  } else {
    el.style.right = '50%';
    el.style.left = 'auto';
    el.style.width = `${(magnitude * 50).toFixed(1)}%`;
  }
}

export function hideWindIndicator(): void {
  ensureWindElements();
  if ($windIndicator) $windIndicator.classList.add('hidden');
}

export function resetWindHint(): void {
  windHintShown = false;
}


// ===== Connection UI (from connection_ui) =====

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


// ===== Waiting tips (from waiting_tips) =====

export function initWaitingTips(): () => void {
  const toggle = document.getElementById('waiting-tips-toggle');
  const body = document.getElementById('waiting-tips-body');
  const summary = document.getElementById('waiting-tips-summary');
  if (!toggle || !body) return () => {};

  toggle.addEventListener('click', () => {
    body.classList.toggle('hidden');
    toggle.setAttribute('aria-expanded', body.classList.contains('hidden') ? 'false' : 'true');
  });

  const intervalId = setInterval(() => {
    if (getState().phase !== 'waiting' || !summary) return;
    const n = Math.max(1, getState().players.length);
    const cd = (calculateCooldown(n) / 1000).toFixed(1);
    summary.textContent = `当前 ${n} 人 · 冷却约 ${cd} 秒`;
  }, 500);

  return () => clearInterval(intervalId);
}
