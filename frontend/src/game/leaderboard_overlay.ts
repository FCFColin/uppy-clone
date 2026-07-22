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

/** Hide the overlay. Safe to call when already closed. */
export function closeLeaderboardOverlay(): void {
  if (!overlay) return;
  overlay.hidden = true;
  open = false;
}

/** Render the shared leaderboard into the overlay and reveal it. */
export async function openLeaderboardOverlay(): Promise<void> {
  if (!overlay || !contentEl) return;
  open = true;
  overlay.hidden = false;
  // Re-render on every open so the data is fresh and any prior scope selection
  // resets to the default global view.
  await renderLeaderboard(contentEl, {
    showBackToLobby: false,
    onClose: closeLeaderboardOverlay,
  });
  // Focus the close button so ESC / keyboard users have a clear anchor.
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
  // Intercept Escape (close) and Space/Enter (prevent the in-game tap handler
  // in window_events.ts from firing while the overlay is up). Capture phase so
  // we run before the bubble-phase game keydown listener on document.
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

/**
 * Bind the overlay's close affordances (× button, backdrop click, ESC) and the
 * HUD「查看排行榜」entry button. Idempotent — safe to call once at boot.
 */
export function initLeaderboardOverlay(): void {
  if (bound) return;
  bound = true;

  overlay = document.getElementById(OVERLAY_ID);
  contentEl = document.getElementById(CONTENT_ID);
  if (!overlay || !contentEl) return;

  // Close on × button and backdrop click via a single delegated listener.
  overlay.addEventListener('click', onCloseAction);
  // Capture phase so we intercept before the game's tap keydown handler.
  document.addEventListener('keydown', onKeydown, { capture: true });

  injectViewButtonIcon();
  const viewBtn = document.getElementById(VIEW_BTN_ID);
  viewBtn?.addEventListener('click', () => {
    void openLeaderboardOverlay();
  });
}
