export {};

import { apiFetch, establishGameSession, normalizeAuthHost, sessionErrorMessage } from './shared/network/network.js';
import { initCollapsibleLeaderboard } from './index_leaderboard.js';
import { ROOM_CODE_RE } from './game/lobby_match.js';
import { initBgParticles } from './bg_particles.js';
import { initHomepageStats } from './homepage_stats.js';
import { initNavigation } from './shared/ui/ui.js';
import { Zap, ArrowRight, Trophy, MousePointer, Timer, Users } from './icons.js';
import { t } from './i18n/t.js';
import { applyTranslations, initLanguageSwitcher } from './i18n/index.js';

normalizeAuthHost();
applyTranslations();
initLanguageSwitcher();
initNavigation();
initCollapsibleLeaderboard();
initBgParticles();
initHomepageStats();
initHeroIcons();
initBentoIcons();
initRowTwoIcons();
initFormIcons();
initGuideIcons();
initScrollReveal();

function initBentoIcons(): void {
  const iconMain = document.querySelector('.bento-icon-main');
  if (iconMain) {
    iconMain.innerHTML = MousePointer({ size: 40, color: '#ff6b8a', strokeWidth: 1.8 });
  }

  const iconTimer = document.querySelector('.bento-icon-timer');
  if (iconTimer) {
    iconTimer.innerHTML = Timer({ size: 24, color: '#60a5fa', strokeWidth: 2 });
  }

  const iconUsers = document.querySelector('.bento-icon-users');
  if (iconUsers) {
    iconUsers.innerHTML = Users({ size: 24, color: '#c4b5fd', strokeWidth: 2 });
  }

  const iconTrophy = document.querySelector('.bento-icon-trophy');
  if (iconTrophy) {
    iconTrophy.innerHTML = Trophy({ size: 24, color: '#fbbf24', strokeWidth: 2 });
  }
}

function initRowTwoIcons(): void {
  const lbIcon = document.querySelector('.bento-icon-lb');
  if (lbIcon) {
    lbIcon.innerHTML = Trophy({ size: 20, color: '#fbbf24', strokeWidth: 2 });
  }

  const moreIcon = document.querySelector('.panel-more-icon');
  if (moreIcon) {
    moreIcon.innerHTML = ArrowRight({ size: 14, color: '#ff8fa3', strokeWidth: 2.5 });
  }

  const browseIcon = document.querySelector('.btn-browse-icon');
  if (browseIcon) {
    browseIcon.innerHTML = ArrowRight({ size: 16, color: '#1a1025', strokeWidth: 2.5 });
  }
}

function initFormIcons(): void {
  const joinBtnTrailing = document.querySelector('#join-code-btn .btn-icon-trailing');
  if (joinBtnTrailing) {
    joinBtnTrailing.innerHTML = ArrowRight({ size: 16, color: 'white', strokeWidth: 2.5 });
  }
}

function initGuideIcons(): void {
  const headingIcon = document.querySelector('.guide-heading-icon');
  if (headingIcon) {
    headingIcon.innerHTML = Zap({ size: 24, color: '#ff6b8a', strokeWidth: 2 });
  }

  const pointerIcon = document.querySelector('.guide-icon-svg.pointer');
  if (pointerIcon) {
    pointerIcon.innerHTML = MousePointer({ size: 28, color: '#ff6b8a', strokeWidth: 1.8 });
  }

  const timerIcon = document.querySelector('.guide-icon-svg.timer');
  if (timerIcon) {
    timerIcon.innerHTML = Timer({ size: 28, color: '#60a5fa', strokeWidth: 1.8 });
  }

  const usersIcon = document.querySelector('.guide-icon-svg.users');
  if (usersIcon) {
    usersIcon.innerHTML = Users({ size: 28, color: '#4ade80', strokeWidth: 1.8 });
  }

  const trophyIcon = document.querySelector('.guide-icon-svg.trophy');
  if (trophyIcon) {
    trophyIcon.innerHTML = Trophy({ size: 28, color: '#c4b5fd', strokeWidth: 1.8 });
  }
}

const errorMsg = document.getElementById('error-msg');
const quickplayBtn = document.getElementById('quickplay-btn') as HTMLButtonElement | null;
const joinCodeBtn = document.getElementById('join-code-btn') as HTMLButtonElement | null;

function initHeroIcons(): void {
  const quickplay = document.getElementById('quickplay-btn');
  if (quickplay) {
    const leading = quickplay.querySelector('.btn-icon-leading');
    const trailing = quickplay.querySelector('.btn-icon-trailing');
    if (leading) leading.innerHTML = Zap({ size: 18, color: 'white', strokeWidth: 2.5 });
    if (trailing) trailing.innerHTML = ArrowRight({ size: 18, color: 'white', strokeWidth: 2.5 });
  }

  const bestPillIcon = document.querySelector('.stat-best .pill-icon');
  if (bestPillIcon) {
    bestPillIcon.innerHTML = Trophy({ size: 16, color: '#fbbf24', strokeWidth: 2 });
  }
}

function setButtonText(btn: HTMLButtonElement, text: string): void {
  const textSpan = btn.querySelector('.btn-text');
  if (textSpan) {
    textSpan.textContent = text;
  } else {
    btn.textContent = text;
  }
}

function showError(message: string): void {
  if (errorMsg) {
    errorMsg.textContent = message;
    errorMsg.style.display = 'block';
  }
}

function resetButton(btn: HTMLButtonElement, text: string): void {
  btn.disabled = false;
  setButtonText(btn, text);
}

async function quickPlay(): Promise<void> {
  if (!quickplayBtn) return;
  quickplayBtn.disabled = true;
  setButtonText(quickplayBtn, t('index.joining'));
  try {
    const session = await establishGameSession();
    if (!session.ok) {
      showError(sessionErrorMessage(session));
      resetButton(quickplayBtn, t('index.quick_start'));
      return;
    }
    const matchRes: Response = await apiFetch('/api/v1/registry/match', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!matchRes.ok) {
      let msg = t('error.match_room');
      if (matchRes.status === 401) msg = t('error.login_expired');
      else if (matchRes.status === 404) msg = t('error.match_unavailable');
      else if (matchRes.status >= 500) msg = t('error.server_busy');
      showError(msg);
      resetButton(quickplayBtn, t('index.quick_start'));
      return;
    }
    const matchData: { lobbyCode?: string } = await matchRes.json();
    if (!matchData.lobbyCode) {
      showError(t('error.match_room'));
      resetButton(quickplayBtn, t('index.quick_start'));
      return;
    }
    sessionStorage.setItem('uppy-auth-ready', '1');
    sessionStorage.setItem('uppy-fresh-match', matchData.lobbyCode);
    window.location.href = `/play.html?code=${encodeURIComponent(matchData.lobbyCode)}`;
  } catch {
    showError(t('error.network_check'));
    resetButton(quickplayBtn, t('index.quick_start'));
  }
}

async function joinByCode(): Promise<void> {
  const inputEl = document.getElementById('join-code-input') as HTMLInputElement | null;
  const errorEl = document.getElementById('join-code-error');
  if (!inputEl || !errorEl || !joinCodeBtn) return;
  const code: string = inputEl.value.trim().toUpperCase();
  if (!ROOM_CODE_RE.test(code)) {
    errorEl.textContent = t('error.room_invalid_code');
    errorEl.classList.remove('hidden');
    return;
  }
  errorEl.classList.add('hidden');
  joinCodeBtn.disabled = true;
  setButtonText(joinCodeBtn, t('index.joining'));
  try {
    const res: Response = await apiFetch(`/api/v1/registry/check/${code}`);
    if (res.status === 404) {
      errorEl.textContent = t('error.room_not_found');
      errorEl.classList.remove('hidden');
      return;
    }
    if (!res.ok) {
      errorEl.textContent = t('error.server_retry');
      errorEl.classList.remove('hidden');
      return;
    }
    const data: { full?: boolean } = await res.json();
    if (data.full) {
      errorEl.textContent = t('error.room_full');
      errorEl.classList.remove('hidden');
      return;
    }
    const session = await establishGameSession();
    if (!session.ok) {
      errorEl.textContent = sessionErrorMessage(session);
      errorEl.classList.remove('hidden');
      return;
    }
    sessionStorage.setItem('uppy-auth-ready', '1');
    sessionStorage.setItem('uppy-fresh-match', code);
    window.location.href = `/play.html?code=${code}`;
  } catch {
    errorEl.textContent = t('error.network_retry');
    errorEl.classList.remove('hidden');
  } finally {
    joinCodeBtn.disabled = false;
    setButtonText(joinCodeBtn, t('index.join'));
  }
}

if (quickplayBtn) quickplayBtn.addEventListener('click', quickPlay);
if (joinCodeBtn) joinCodeBtn.addEventListener('click', joinByCode);
document.getElementById('join-code-input')?.addEventListener('keydown', (e: KeyboardEvent) => {
  if (e.key === 'Enter') joinByCode();
});

function initScrollReveal(): void {
  const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (prefersReducedMotion) {
    document.querySelectorAll('[data-reveal]').forEach(el => {
      el.classList.add('revealed');
    });
    return;
  }

  const revealElements = document.querySelectorAll<HTMLElement>('[data-reveal]');
  if (revealElements.length === 0) return;

  const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        entry.target.classList.add('revealed');
        observer.unobserve(entry.target);
      }
    });
  }, {
    threshold: 0.15,
    rootMargin: '0px 0px -50px 0px'
  });

  revealElements.forEach(el => {
    observer.observe(el);
  });
}
