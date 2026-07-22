import { bindWindowEvents } from './window_events.js';
import { boot } from './lifecycle.js';
import { initBgParticles } from '../bg_particles.js';
import { initLeaderboardOverlay } from './leaderboard_overlay.js';

initBgParticles();
bindWindowEvents();
boot();
initLeaderboardOverlay();
