import { Globe, Gamepad2, Menu } from '../../icons.js';

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
