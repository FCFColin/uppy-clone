import { t } from '../i18n/t.js';
import { dispatch, getState } from './state.js';
import { showToast, playCountdownTick, playReadySound, vibrate, isMuted, initMuteToggle } from '../shared/ui/ui.js';
import { calculateCooldown } from './message_codec.js';
import { isTutorialDone } from '../shared/data/cookies.js';
import { PHYSICS } from '../shared/game/constants.js';
import { routeConnectionError, type EntryFullScreenErrorOptions } from './entry_flow.js';
import { icons } from '../icons.js';
import { initLeaderboardOverlay } from './leaderboard_overlay.js';

import { NICKNAME_ADJECTIVES, NICKNAME_CATEGORIES } from '../shared/assets/nickname_pools_gen.js';
import { getLanguage } from '../i18n/index.js';
import { NICKNAME_ADJECTIVES_EN, NICKNAME_CATEGORIES_EN } from '../shared/assets/nickname_pools_en.js';

export const $waitingScreen: HTMLElement = document.getElementById('waiting-screen')!;
export const $endedScreen: HTMLElement = document.getElementById('ended-screen')!;
export const $gameHud: HTMLElement = document.getElementById('game-hud')!;
export const $cooldownIndicator: HTMLElement = document.getElementById('cooldown-indicator')!;
export const $cooldownBar: HTMLElement = document.getElementById('cooldown-bar')!;
export const $cooldownText: HTMLElement = document.getElementById('cooldown-text')!;
export const $lobbyCode: HTMLElement = document.getElementById('lobby-code')!;
export const $hudCode: HTMLElement = document.getElementById('hud-code')!;
export const $hudScore: HTMLElement = document.getElementById('hud-score')!;
export const $hudTimer: HTMLElement | null = document.getElementById('hud-timer');
export const $hudPlayers: HTMLElement = document.getElementById('hud-players')!;
export const $hudPlayerList: HTMLElement = document.getElementById('hud-player-list')!;
export const $finalScore: HTMLElement = document.getElementById('final-score')!;
export const $endPlayerList: HTMLElement = document.getElementById('end-player-list')!;
export const $playerListWaiting: HTMLElement = document.getElementById('player-list-waiting')!;
export const $copyCodeBtn: HTMLElement | null = document.getElementById('copy-code-btn');
export const $hudCopyBtn: HTMLElement | null = document.getElementById('hud-copy-btn');
export const $leaveGameBtn: HTMLElement | null = document.getElementById('leave-game-btn');
export const $nicknameSetupScreen: HTMLElement | null = document.getElementById('nickname-setup-screen');
export const $setupNicknameInput: HTMLInputElement | null = document.getElementById(
  'setup-nickname-input',
) as HTMLInputElement | null;

let gameStartTime: number | null = null;
let hudBound = false;

export function pickRandomNickname(): string {
  const isEn = getLanguage() === 'en';
  const adjPool = isEn ? NICKNAME_ADJECTIVES_EN : NICKNAME_ADJECTIVES;
  const catPool = isEn ? NICKNAME_CATEGORIES_EN : NICKNAME_CATEGORIES;
  const adj: string = adjPool[Math.floor(Math.random() * adjPool.length)]!;
  const category: readonly string[] = catPool[Math.floor(Math.random() * catPool.length)]!;
  const noun: string = category[Math.floor(Math.random() * category.length)]!;
  return adj + noun;
}

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

export async function copyCode(): Promise<void> {
  const url: string = `${window.location.origin}/play.html?code=${getState().lobbyCode}`;
  try {
    await navigator.clipboard.writeText(url);
    showToast(t('error.copy_success'));
  } catch {
    showToast(t('error.copy_failed'));
  }
}

export function showFallbackErrorScreen(message: string): void {
  if (document.getElementById('game-fallback-error')) return;
  const overlay: HTMLDivElement = document.createElement('div');
  overlay.id = 'game-fallback-error';
  overlay.style.cssText =
    'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.8);z-index:99999;display:flex;align-items:center;justify-content:center;flex-direction:column;color:#fff;font-family:sans-serif;';

  const h2: HTMLHeadingElement = document.createElement('h2');
  h2.style.marginBottom = '1rem';
  h2.textContent = '\u{1F635} ' + t('error.error_title');

  const p: HTMLParagraphElement = document.createElement('p');
  p.style.cssText = 'margin-bottom:1.5rem;color:#ccc;';
  p.textContent = message;

  const btn: HTMLButtonElement = document.createElement('button');
  btn.style.cssText =
    'padding:0.8rem 2rem;font-size:1rem;cursor:pointer;border:none;border-radius:8px;background:#0f3460;color:#fff;';
  btn.textContent = t('error.refresh_page');
  btn.onclick = () => location.reload();

  overlay.appendChild(h2);
  overlay.appendChild(p);
  overlay.appendChild(btn);
  document.body.appendChild(overlay);
}

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
    $cooldownText.textContent = t('play.tap');
    if (!wasReady) {
      wasReady = true;
      playReadySound();
      vibrate(30);
    }
  }
}

const WIND_CLAMP = PHYSICS.WIND_CLAMP;
const STRONG_WIND_THRESHOLD = 0.55;

let $windIndicator: HTMLElement | null = null;
let $windDirection: HTMLElement | null = null;
let $windMeterFill: HTMLElement | null = null;
let $windStrength: HTMLElement | null = null;
let $windPercent: HTMLElement | null = null;
let $windFirstHint: HTMLElement | null = null;
let windHintShown = false;

function ensureWindElements(): void {
  if ($windIndicator && !$windIndicator.isConnected) {
    $windIndicator = null;
    $windDirection = null;
    $windMeterFill = null;
    $windStrength = null;
    $windPercent = null;
    $windFirstHint = null;
  }
  if (!$windIndicator) $windIndicator = document.getElementById('wind-indicator');
  if (!$windDirection) $windDirection = document.getElementById('wind-direction');
  if (!$windMeterFill) $windMeterFill = document.getElementById('wind-meter-fill');
  if (!$windStrength) $windStrength = document.getElementById('wind-strength');
  if (!$windPercent) $windPercent = document.getElementById('wind-percent');
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
  $windDirection.style.color = magnitude >= STRONG_WIND_THRESHOLD ? '#ffb4c4' : '#7dd3fc';
  $windStrength.textContent = `${pct}%`;
  if ($windPercent) $windPercent.textContent = `${pct}%`;

  $windMeterFill.classList.toggle('strong', magnitude >= STRONG_WIND_THRESHOLD);
  applyWindMeterStyle($windMeterFill, clamped, magnitude, isCalm);

  const dirLabel = windDirLabel(clamped, isCalm);
  $windIndicator.title = t('wind.title', { dir: dirLabel, pct });

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
  if (isCalm) return t('wind.calm');
  return clamped >= 0 ? t('wind.east') : t('wind.west');
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

export function bindReconnectRetry(retryFn: () => void): void {
  if (retryBound) return;
  retryBound = true;
  const btn = document.getElementById('reconnect-retry-btn');
  btn?.addEventListener('click', () => retryFn());
}

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
    summary.textContent = t('play.player_count', { count: n, cd });
  }, 500);

  return () => clearInterval(intervalId);
}

function formatGameTime(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
}

export function updateHudTimer(): void {
  if (!$hudTimer) return;
  if (getState().phase !== 'playing' || gameStartTime === null) {
    $hudTimer.textContent = '00:00';
    return;
  }
  const elapsed = Date.now() - gameStartTime;
  $hudTimer.textContent = formatGameTime(elapsed);
}

export function onGamePhaseChange(): void {
  if (getState().phase === 'playing' && gameStartTime === null) {
    gameStartTime = Date.now();
  } else if (getState().phase === 'waiting' || getState().phase === 'ended') {
    if (getState().phase === 'waiting') {
      gameStartTime = null;
    }
  }
}

function injectHudIcons(): void {
  const scoreIcon = document.querySelector('.hud-icon-score');
  if (scoreIcon) scoreIcon.innerHTML = icons.Trophy({ size: 18, color: '#fbbf24' });

  const timerIcon = document.querySelector('.hud-icon-timer');
  if (timerIcon) timerIcon.innerHTML = icons.Timer({ size: 18 });

  const playersIcon = document.querySelector('.hud-icon-users');
  if (playersIcon) playersIcon.innerHTML = icons.Users({ size: 18 });

  const roomCodeIcon = document.querySelector('.hud-icon-key');
  if (roomCodeIcon) roomCodeIcon.innerHTML = icons.Key({ size: 18, color: '#22d3ee' });

  if ($hudCopyBtn) $hudCopyBtn.innerHTML = icons.Copy({ size: 18 });
  if ($leaveGameBtn) $leaveGameBtn.innerHTML = icons.LogOut({ size: 18 });
  if ($copyCodeBtn && !$copyCodeBtn.innerHTML.trim()) {
    $copyCodeBtn.innerHTML = icons.Copy({ size: 18 });
  }

  const trophyIcon = document.querySelector('.ended-trophy-icon');
  if (trophyIcon) trophyIcon.innerHTML = icons.Trophy({ size: 48, color: '#fbbf24' });

  const restartBtnIcon = document.querySelector('.restart-btn-icon');
  if (restartBtnIcon) restartBtnIcon.innerHTML = icons.RotateCcw({ size: 20 });

  const homeBtnIcon = document.querySelector('.home-btn-icon');
  if (homeBtnIcon) homeBtnIcon.innerHTML = icons.Home({ size: 20 });

  const muteBtn = document.getElementById('mute-toggle');
  if (muteBtn) {
    muteBtn.innerHTML = isMuted() ? icons.VolumeX({ size: 18 }) : icons.Volume2({ size: 18 });
  }

  const leaderboardBtn = document.getElementById('view-leaderboard-btn');
  if (leaderboardBtn) leaderboardBtn.innerHTML = icons.Trophy({ size: 18, color: '#a78bfa' });

  const menuBtn = document.getElementById('hud-menu-btn');
  if (menuBtn) {
    menuBtn.innerHTML = icons.Menu({ size: 18 });
  }
}

function bindHudEvents(): void {
  if (hudBound) return;
  hudBound = true;

  if ($leaveGameBtn) {
    $leaveGameBtn.addEventListener('click', () => {
      window.location.href = '/';
    });
  }

  initMuteToggle();
  initLeaderboardOverlay();

  const menuBtn = document.getElementById('hud-menu-btn');
  if (menuBtn) {
    menuBtn.addEventListener('click', () => {
    });
  }
}

export function initHud(): void {
  injectHudIcons();
  bindHudEvents();
  gameStartTime = null;
}
