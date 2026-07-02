import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { state } from './state.js';

const mocks = vi.hoisted(() => {
  const bar = {
    style: { width: '100%' },
    classList: {
      add: vi.fn(),
      remove: vi.fn(),
      contains: vi.fn(() => false),
    },
  };
  const text = { textContent: '' };
  return { bar, text };
});

vi.mock('./ui_elements.js', () => ({
  $cooldownBar: mocks.bar,
  $cooldownText: mocks.text,
}));
vi.mock('../shared/ui/audio.js', () => ({
  playReadySound: vi.fn(),
  vibrate: vi.fn(),
}));

import { startCooldownUpdater, stopCooldownUpdater, updateCooldownBar } from './ui_cooldown.js';

describe('ui_cooldown', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mocks.bar.style.width = '100%';
    mocks.text.textContent = '';
    mocks.bar.classList.add.mockClear();
    mocks.bar.classList.remove.mockClear();
    state.phase = 'playing';
    state.myCooldownEnd = Date.now() + 2000;
    state.players = [{ playerIndex: 0, cooldownEndTime: 0, palette: 0, scoreContribution: 0, nickname: 'a' }];
  });

  afterEach(() => {
    stopCooldownUpdater();
    vi.useRealTimers();
  });

  it('updates bar width while cooling down', () => {
    updateCooldownBar();
    expect(mocks.bar.style.width).not.toBe('0%');
    expect(mocks.bar.classList.remove).toHaveBeenCalledWith('ready');
  });

  it('shows Tap when cooldown elapsed', () => {
    state.myCooldownEnd = Date.now() - 1;
    updateCooldownBar();
    expect(mocks.text.textContent).toBe('点击！');
    expect(mocks.bar.classList.add).toHaveBeenCalledWith('ready');
  });

  it('interval updater runs at 10Hz during playing phase', () => {
    startCooldownUpdater();
    vi.advanceTimersByTime(300);
    expect(mocks.text.textContent).toMatch(/s|点击！/);
  });

  it('does nothing when phase is not playing', () => {
    state.phase = 'waiting';
    state.myCooldownEnd = Date.now() + 5000;
    updateCooldownBar();
    expect(mocks.bar.style.width).toBe('100%');
  });
});
