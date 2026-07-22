export {};

import { renderLeaderboard } from './leaderboard/shared.js';
import { initBgParticles } from './bg_particles.js';
import { initNavigation } from './shared/ui/nav.js';
import { Trophy, ArrowRight } from './icons.js';

initNavigation();
initBgParticles();

const root = document.getElementById('leaderboard-root');
if (root) {
  void renderLeaderboard(root, { showBackToLobby: true });
}
// Run after renderLeaderboard's synchronous DOM build so the shared module's
//「返回大厅」button exists when we inject its trailing arrow icon.
initPageIcons();

function initPageIcons(): void {
  const eyebrowIcon = document.querySelector('.lb-eyebrow-icon');
  if (eyebrowIcon) {
    eyebrowIcon.innerHTML = Trophy({ size: 16, color: '#ff8fa3', strokeWidth: 2 });
  }

  // The「返回大厅」button is rendered by the shared module; inject the trailing
  // arrow icon once it exists in the DOM.
  const backHallTrailing = document.querySelector('.lb-action-btn .btn-icon-trailing');
  if (backHallTrailing) {
    backHallTrailing.innerHTML = ArrowRight({ size: 18, color: 'white', strokeWidth: 2.5 });
  }
}
