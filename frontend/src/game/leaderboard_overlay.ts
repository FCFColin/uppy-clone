import { renderLeaderboard } from '../leaderboard/shared.js';
import { Trophy } from '../icons.js';

const OVERLAY_ID = 'leaderboard-overlay';
const CONTENT_ID = 'leaderboard-overlay-content';
const CLOSE_ACTION = 'close-leaderboard';
const VIEW_BTN_ID = 'view-leaderboard-btn';

let overlay: HTMLElement | null = null;
let contentEl: HTMLElement | null = null;
let bound = false;
let open = false;

function isOverlayOpen(): boolean {
  return open;
}

export function closeLeaderboardOverlay(): void {
  if (!overlay) return;
  overlay.hidden = true;
  open = false;
}

export async function openLeaderboardOverlay(): Promise<void> {
  if (!overlay || !contentEl) return;
  open = true;
  overlay.hidden = false;
  await renderLeaderboard(contentEl, {
    showBackToLobby: false,
    onClose: closeLeaderboardOverlay,
  });
  const closeBtn = overlay.querySelector<HTMLButtonElement>('.lb-overlay-close');
  closeBtn?.focus();
}

function onCloseAction(e: Event): void {
  const target = e.target as HTMLElement | null;
  if (!target) return;
  if (target.dataset.action === CLOSE_ACTION) {
    e.preventDefault();
    closeLeaderboardOverlay();
  }
}

function onKeydown(e: KeyboardEvent): void {
  if (!isOverlayOpen()) return;
  if (e.key === 'Escape') {
    e.preventDefault();
    e.stopImmediatePropagation();
    closeLeaderboardOverlay();
  } else if (e.key === ' ' || e.key === 'Enter') {
    e.preventDefault();
    e.stopImmediatePropagation();
  }
}

function injectViewButtonIcon(): void {
  const btn = document.getElementById(VIEW_BTN_ID);
  if (btn && !btn.innerHTML.trim()) {
    btn.innerHTML = Trophy({ size: 16, color: '#fbbf24' });
  }
}

export function initLeaderboardOverlay(): void {
  if (bound) return;
  bound = true;

  overlay = document.getElementById(OVERLAY_ID);
  contentEl = document.getElementById(CONTENT_ID);
  if (!overlay || !contentEl) return;

  overlay.addEventListener('click', onCloseAction);
  document.addEventListener('keydown', onKeydown, { capture: true });

  injectViewButtonIcon();
  const viewBtn = document.getElementById(VIEW_BTN_ID);
  viewBtn?.addEventListener('click', () => {
    void openLeaderboardOverlay();
  });
}
