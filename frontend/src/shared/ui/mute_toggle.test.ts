import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { initMuteToggle, updateMuteToggleIcon } from './mute_toggle.js';

const mockIsMuted = vi.hoisted(() => vi.fn(() => false));
const mockToggleMute = vi.hoisted(() => vi.fn(() => false));

vi.mock('./audio.js', () => ({
  isMuted: mockIsMuted,
  toggleMute: mockToggleMute,
}));

describe('mute_toggle', () => {
  let button: HTMLButtonElement;
  let storage: Record<string, string>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockIsMuted.mockReturnValue(false);
    mockToggleMute.mockReturnValue(false);

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

  it('initMuteToggle 按钮不存在时不抛错', () => {
    button.remove();
    expect(() => initMuteToggle()).not.toThrow();
  });

  it('initMuteToggle 按钮存在时绑定 click 事件并刷新图标', () => {
    initMuteToggle();
    expect(button.innerHTML).toContain('<svg');
    expect(button.innerHTML).toContain('<path');
    button.click();
    expect(mockToggleMute).toHaveBeenCalledTimes(1);
  });

  it('initMuteToggle 多次调用幂等（不重复绑定）', () => {
    initMuteToggle();
    initMuteToggle();
    initMuteToggle();
    button.click();
    expect(mockToggleMute).toHaveBeenCalledTimes(1);
  });

  it('点击按钮调用 toggleMute 并更新图标', () => {
    initMuteToggle();
    mockIsMuted.mockReturnValue(true);
    button.click();
    expect(mockToggleMute).toHaveBeenCalledTimes(1);
    expect(button.innerHTML).toContain('<svg');
    expect(button.innerHTML).toContain('<line');
  });

  it('updateMuteToggleIcon 静音时显示 VolumeX 图标（带line划线）', () => {
    mockIsMuted.mockReturnValue(true);
    updateMuteToggleIcon();
    expect(button.innerHTML).toContain('<svg');
    expect(button.innerHTML).toContain('<line');
  });

  it('updateMuteToggleIcon 非静音时显示 Volume2 图标（带path声波）', () => {
    mockIsMuted.mockReturnValue(false);
    updateMuteToggleIcon();
    expect(button.innerHTML).toContain('<svg');
    expect(button.innerHTML).toContain('<path');
    expect(button.innerHTML).not.toContain('<line');
  });

  it('updateMuteToggleIcon 按钮不存在时不抛错', () => {
    button.remove();
    expect(() => updateMuteToggleIcon()).not.toThrow();
  });
});
