import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { state } from './state_types.js';
import { updateWindIndicator, hideWindIndicator } from './ui_wind.js';

describe('ui_wind', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <div id="wind-indicator" class="wind-indicator hidden">
        <span id="wind-direction" class="wind-direction">→</span>
        <div class="wind-meter"><div class="wind-meter-track">
          <div id="wind-meter-fill" class="wind-meter-fill"></div>
        </div></div>
        <span id="wind-strength" class="wind-strength">0%</span>
      </div>
    `;
    state.phase = 'playing';
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('shows left wind with strength percentage and meter fill', () => {
    updateWindIndicator(-0.5);
    const indicator = document.getElementById('wind-indicator')!;
    const direction = document.getElementById('wind-direction') as HTMLElement;
    const fill = document.getElementById('wind-meter-fill') as HTMLElement;
    const strength = document.getElementById('wind-strength') as HTMLElement;

    expect(indicator.classList.contains('hidden')).toBe(false);
    expect(direction.textContent).toBe('←');
    expect(strength.textContent).toBe('77%');
    expect(fill.style.right).toBe('50%');
    expect(parseFloat(fill.style.width)).toBeGreaterThan(35);
  });

  it('shows calm state for near-zero wind', () => {
    updateWindIndicator(0.02);
    const direction = document.getElementById('wind-direction') as HTMLElement;
    const strength = document.getElementById('wind-strength') as HTMLElement;
    const fill = document.getElementById('wind-meter-fill') as HTMLElement;

    expect(direction.textContent).toBe('·');
    expect(strength.textContent).toBe('3%');
    expect(fill.style.width).toBe('0%');
  });

  it('marks strong wind with strong class', () => {
    updateWindIndicator(0.6);
    const fill = document.getElementById('wind-meter-fill') as HTMLElement;
    expect(fill.classList.contains('strong')).toBe(true);
  });

  it('hides indicator outside playing phase', () => {
    state.phase = 'waiting';
    hideWindIndicator();
    const indicator = document.getElementById('wind-indicator')!;
    expect(indicator.classList.contains('hidden')).toBe(true);
  });
});
