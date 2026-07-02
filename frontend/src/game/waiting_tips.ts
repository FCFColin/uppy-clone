import { calculateCooldown } from './message_codec.js';
import { state } from './state_types.js';

export function initWaitingTips(): void {
  const toggle = document.getElementById('waiting-tips-toggle');
  const body = document.getElementById('waiting-tips-body');
  const summary = document.getElementById('waiting-tips-summary');
  if (!toggle || !body) return;

  toggle.addEventListener('click', () => {
    body.classList.toggle('hidden');
    toggle.setAttribute('aria-expanded', body.classList.contains('hidden') ? 'false' : 'true');
  });

  setInterval(() => {
    if (state.phase !== 'waiting' || !summary) return;
    const n = Math.max(1, state.players.length);
    const cd = (calculateCooldown(n) / 1000).toFixed(1);
    summary.textContent = `当前 ${n} 人 · 冷却约 ${cd} 秒`;
  }, 500);
}
