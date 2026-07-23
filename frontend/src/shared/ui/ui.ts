import { Globe, Gamepad2, Menu, VolumeX, Volume2 } from '../../icons.js';
import { t } from '../../i18n/t.js';
import { apiFetch } from '../network/network.js';

let audioCtx: AudioContext | null = null;

function ctx(): AudioContext | null {
  if (typeof window === 'undefined') return null;
  if (!audioCtx) {
    try {
      audioCtx = new AudioContext();
    } catch {
      return null;
    }
  }
  return audioCtx;
}

const MUTE_STORAGE_KEY = 'uppy-audio-muted';

export function isMuted(): boolean {
  try {
    return localStorage.getItem(MUTE_STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

export function setMuted(muted: boolean): void {
  try {
    if (muted) {
      localStorage.setItem(MUTE_STORAGE_KEY, '1');
    } else {
      localStorage.removeItem(MUTE_STORAGE_KEY);
    }
  } catch {
    // ignore — storage may be unavailable (private browsing etc.)
  }
}

export function toggleMute(): boolean {
  const newMuted = !isMuted();
  setMuted(newMuted);
  return newMuted;
}

function playSoundFile(path: string): void {
  try {
    const audio = new Audio(path);
    audio.play().catch(() => {
      // ignore — audio playback may fail without user interaction
    });
  } catch {
    // ignore — audio playback may fail without user interaction
  }
}

export function playTapSound(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/tap.ogg');
}
export function playReadySound(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/ready.ogg');
}
export function playGameOverSound(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/gameover.ogg');
}
export function playCountdownTick(): void {
  if (isMuted()) return;
  playSoundFile('/assets/sounds/countdown.ogg');
}

export function vibrate(pattern: number | number[]): void {
  if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  try {
    navigator.vibrate?.(pattern);
  } catch {
    // ignore — vibration may not be supported on this device
  }
}

export function resumeAudioContext(): void {
  void ctx()?.resume();
}

const MUTE_TOGGLE_ID = 'mute-toggle';
const BOUND_ATTR = 'data-bound';

function getMuteButton(): HTMLElement | null {
  return document.getElementById(MUTE_TOGGLE_ID);
}

export function initMuteToggle(): void {
  const btn = getMuteButton();
  if (!btn) return;
  if (btn.hasAttribute(BOUND_ATTR)) return;
  btn.setAttribute(BOUND_ATTR, 'true');
  btn.addEventListener('click', () => {
    toggleMute();
    updateMuteToggleIcon();
  });
  updateMuteToggleIcon();
}

export function updateMuteToggleIcon(): void {
  const btn = getMuteButton();
  if (!btn) return;
  btn.innerHTML = isMuted() ? VolumeX({ size: 18 }) : Volume2({ size: 18 });
  btn.setAttribute('aria-label', isMuted() ? t('mute.unmute') : t('mute.mute'));
  btn.setAttribute('title', isMuted() ? t('mute.unmute') : t('mute.mute'));
}

export function safeGetItem(key: string, storage: Storage = localStorage): string | null {
  try {
    return storage.getItem(key);
  } catch {
    return null;
  }
}

export function safeSetItem(key: string, value: string, storage: Storage = localStorage): void {
  try {
    storage.setItem(key, value);
  } catch {
    // ignore — storage may be unavailable (private browsing etc.)
  }
}

let toastTimer: ReturnType<typeof setTimeout> | null = null;

export function showToast(message: string, durationMs = 2000): void {
  let el = document.getElementById('app-toast');
  if (!el) {
    el = document.createElement('div');
    el.id = 'app-toast';
    el.className = 'app-toast';
    document.body.appendChild(el);
  }
  el.textContent = message;
  el.classList.add('visible');
  if (toastTimer !== null) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el?.classList.remove('visible');
    toastTimer = null;
  }, durationMs);
}

export function getBalloonLogoSvg(): string {
  return `
    <svg class="logo-balloon" width="28" height="28" viewBox="0 0 28 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id="balloonGradient" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#ff6b8a"/>
          <stop offset="100%" stop-color="#e94560"/>
        </linearGradient>
      </defs>
      <ellipse cx="14" cy="11" rx="9" ry="10" fill="url(#balloonGradient)"/>
      <ellipse cx="11" cy="8" rx="2.5" ry="3" fill="rgba(255,255,255,0.3)"/>
      <polygon points="14,21 12,24 16,24" fill="#cc3d54"/>
      <path d="M14 24 Q16 26 13 27 Q10 28 14 30" stroke="#b8a0c8" stroke-width="1" stroke-linecap="round" fill="none"/>
    </svg>
  `;
}

export function initNavigation(): void {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', setupNavigation);
  } else {
    setupNavigation();
  }
}

function setupNavigation(): void {
  injectNavIcons();
  setupScrollListener();
}

function injectNavIcons(): void {
  const langBtn = document.querySelector('.btn-nav-icon[data-icon="globe"]') as HTMLElement | null;
  const controllerBtn = document.querySelector('.btn-nav-icon[data-icon="gamepad"]') as HTMLElement | null;
  const menuBtn = document.querySelector('.btn-nav-icon[data-icon="menu"]') as HTMLElement | null;
  const logoEmoji = document.querySelector('.nav-logo .logo-emoji') as HTMLElement | null;
  const footerLogoEmoji = document.querySelector('.footer-logo .logo-emoji') as HTMLElement | null;

  if (langBtn) {
    langBtn.innerHTML = Globe({ size: 18, strokeWidth: 2 });
  }
  if (controllerBtn) {
    controllerBtn.innerHTML = Gamepad2({ size: 18, strokeWidth: 2 });
  }
  if (menuBtn) {
    menuBtn.innerHTML = Menu({ size: 20, strokeWidth: 2 });
  }
  if (logoEmoji) {
    logoEmoji.outerHTML = getBalloonLogoSvg();
  }
  if (footerLogoEmoji) {
    footerLogoEmoji.outerHTML = getBalloonLogoSvg();
  }
}

function setupScrollListener(): void {
  const nav = document.querySelector('.top-nav') as HTMLElement | null;
  if (!nav) return;

  const onScroll = (): void => {
    if (window.scrollY > 20) {
      nav.classList.add('scrolled');
    } else {
      nav.classList.remove('scrolled');
    }
  };

  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();
}

export interface LeaderboardEntry {
  rank: number;
  score: number;
  name: string;
}

export type Scope = 'global' | 'weekly';

export function renderLeaderboardEntry(parent: HTMLElement, e: LeaderboardEntry): void {
  const li = document.createElement('li');
  li.className = 'leaderboard-item';

  const rank = document.createElement('span');
  rank.className = 'lb-rank';
  rank.textContent = `#${e.rank}`;

  const score = document.createElement('span');
  score.className = 'lb-score';
  score.textContent = String(e.score);

  const code = document.createElement('span');
  code.className = 'lb-code';
  code.textContent = e.name;

  li.append(rank, score, code);
  parent.appendChild(li);
}

export async function fetchLeaderboard(scope: Scope, limit: number): Promise<LeaderboardEntry[]> {
  const res = await apiFetch(`/api/v1/leaderboard?scope=${scope}&limit=${limit}`, { autoRefresh: false });
  if (!res.ok) throw new Error(`load failed (${res.status})`);
  const data: { entries: LeaderboardEntry[] } = await res.json();
  return data.entries ?? [];
}
