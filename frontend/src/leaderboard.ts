export {};

import { renderLeaderboard } from './leaderboard/shared.js';
import { initBgParticles } from './bg_particles.js';
import { initNavigation } from './shared/ui/ui.js';
import { Trophy, ArrowRight } from './icons.js';
import { applyTranslations, initLanguageSwitcher } from './i18n/index.js';

applyTranslations();
initLanguageSwitcher();
initNavigation();
initBgParticles();

const root = document.getElementById('leaderboard-root');
if (root) {
  void renderLeaderboard(root, { showBackToLobby: true });
}
initPageIcons();

function initPageIcons(): void {
  const eyebrowIcon = document.querySelector('.lb-eyebrow-icon');
  if (eyebrowIcon) {
    eyebrowIcon.innerHTML = Trophy({ size: 16, color: '#ff8fa3', strokeWidth: 2 });
  }

  const backHallTrailing = document.querySelector('.lb-action-btn .btn-icon-trailing');
  if (backHallTrailing) {
    backHallTrailing.innerHTML = ArrowRight({ size: 18, color: 'white', strokeWidth: 2.5 });
  }
}
