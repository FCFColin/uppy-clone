import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { initMuteToggle, isMuted } from './ui.js';

describe('mute_toggle', () => {
  let button: HTMLButtonElement;
  let storage: Record<string, string>;

  beforeEach(() => {
    vi.clearAllMocks();

    storage = {};
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation((key: string) => (key in storage ? storage[key]! : null));
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation((key: string, value: string) => {
      storage[key] = value;
    });
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation((key: string) => {
      delete storage[key];
    });

    button = document.createElement('button');
    button.id = 'mute-toggle';
    button.type = 'button';
    document.body.appendChild(button);
  });

  afterEach(() => {
    button.remove();
  });

  it('initMuteToggle 按钮存在时绑定 click 事件并刷新图标', () => {
    initMuteToggle();
    expect(button.innerHTML).toContain('<svg');
    expect(button.innerHTML).toContain('<path');
    button.click();
    expect(isMuted()).toBe(true);
  });

  it('点击按钮调用 toggleMute 并更新图标', () => {
    storage['uppy-audio-muted'] = '1';
    initMuteToggle();
    expect(button.innerHTML).toContain('<line');
    button.click();
    expect(isMuted()).toBe(false);
    expect(button.innerHTML).toContain('<svg');
    expect(button.innerHTML).toContain('<path');
    expect(button.innerHTML).not.toContain('<line');
  });
});
