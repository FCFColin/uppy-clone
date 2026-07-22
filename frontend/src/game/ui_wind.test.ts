import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { state } from './state.js';
import { updateWindIndicator, hideWindIndicator } from './ui_common.js';

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

  it.each([
    [-0.5, '←', '77%', false, true],
    [0.02, '·', '3%', true, false],
    [0.6, '→', '92%', false, true],
  ] as const)(
    'wind=%f shows direction=%s strength=%s calm=%s strong=%s',
    (wind, expectedDir, expectedStrength, isCalm, isStrong) => {
      updateWindIndicator(wind);
      const indicator = document.getElementById('wind-indicator')!;
      const direction = document.getElementById('wind-direction') as HTMLElement;
      const strength = document.getElementById('wind-strength') as HTMLElement;
      const fill = document.getElementById('wind-meter-fill') as HTMLElement;

      expect(indicator.classList.contains('hidden')).toBe(false);
      expect(direction.textContent).toBe(expectedDir);
      expect(strength.textContent).toBe(expectedStrength);
      if (isCalm) {
        expect(fill.style.width).toBe('0%');
      }
      if (isStrong) {
        expect(fill.classList.contains('strong')).toBe(true);
      }
    },
  );

  it('hides indicator outside playing phase', () => {
    state.phase = 'waiting';
    hideWindIndicator();
    const indicator = document.getElementById('wind-indicator')!;
    expect(indicator.classList.contains('hidden')).toBe(true);
  });
});
