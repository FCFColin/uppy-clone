import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ui_common.ts exports $cooldownBar/$cooldownText as module-level singletons
// captured at import time. We must set up the DOM BEFORE imports so these
// references point to valid elements. The element objects remain valid even
// if later detached from the document (innerHTML replacement in wind tests).
const cooldownEls = vi.hoisted(() => {
  document.body.innerHTML = '<div id="cooldown-bar"></div><div id="cooldown-text"></div>';
  return {
    bar: document.getElementById('cooldown-bar')!,
    text: document.getElementById('cooldown-text')!,
  };
});

import { state } from './state.js';
import { startCooldownUpdater, stopCooldownUpdater, updateCooldownBar, updateWindIndicator, hideWindIndicator } from './ui_common.js';

describe('ui_cooldown', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // Reset state of the module-level singleton elements (don't replace innerHTML,
    // which would create new elements and break the singleton references in ui_common.ts).
    cooldownEls.bar.style.width = '100%';
    cooldownEls.bar.classList.remove('ready');
    cooldownEls.text.textContent = '';
    state.phase = 'playing';
    state.myCooldownEnd = Date.now() + 2000;
    state.players = [{ playerIndex: 0, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'a' }];
  });

  afterEach(() => {
    stopCooldownUpdater();
    vi.useRealTimers();
  });

  it('updates bar width while cooling down, shows Tap when elapsed', () => {
    // Cooling down
    updateCooldownBar();
    expect(cooldownEls.bar.style.width).not.toBe('0%');
    expect(cooldownEls.bar.classList.contains('ready')).toBe(false);
    // Elapsed
    state.myCooldownEnd = Date.now() - 1;
    updateCooldownBar();
    expect(cooldownEls.text.textContent).toBe('点击！');
    expect(cooldownEls.bar.classList.contains('ready')).toBe(true);
  });

  it('interval updater runs at 10Hz during playing phase', () => {
    startCooldownUpdater();
    vi.advanceTimersByTime(300);
    expect(cooldownEls.text.textContent).toMatch(/s|点击！/);
  });

  it('does nothing when phase is not playing', () => {
    state.phase = 'waiting';
    state.myCooldownEnd = Date.now() + 5000;
    updateCooldownBar();
    expect(cooldownEls.bar.style.width).toBe('100%');
  });
});

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
