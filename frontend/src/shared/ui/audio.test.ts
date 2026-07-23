import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  playTapSound,
  playReadySound,
  playGameOverSound,
  playCountdownTick,
  isMuted,
  setMuted,
} from './ui.js';

describe('audio', () => {
  let audioConstructorSpy: ReturnType<typeof vi.fn>;
  let playSpy: ReturnType<typeof vi.fn>;
  let storage: Record<string, string>;

  beforeEach(() => {
    vi.restoreAllMocks();
    delete (window as unknown as Record<string, unknown>).AudioContext;

    storage = {};
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation((key: string) => (key in storage ? storage[key]! : null));
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation((key: string, value: string) => {
      storage[key] = value;
    });
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation((key: string) => {
      delete storage[key];
    });

    playSpy = vi.fn().mockReturnValue(Promise.resolve());
    audioConstructorSpy = vi.fn().mockImplementation(function (this: { src: string; play: typeof playSpy }, path: string) {
      this.src = path;
      this.play = playSpy;
    });
    (window as unknown as Record<string, unknown>).Audio = audioConstructorSpy;
  });

  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).Audio;
  });

  it('isMuted / setMuted / toggleMute 读写 localStorage', () => {
    expect(isMuted()).toBe(false);
    setMuted(true);
    expect(isMuted()).toBe(true);
    expect(storage['uppy-audio-muted']).toBe('1');
    setMuted(false);
    expect(isMuted()).toBe(false);
    expect('uppy-audio-muted' in storage).toBe(false);
  });

  it('isMuted 在 localStorage 抛错时返回 false', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });
    expect(isMuted()).toBe(false);
  });

  it('非静音时 4 个音效函数调用 new Audio 并播放对应 .ogg', () => {
    expect(isMuted()).toBe(false);

    playTapSound();
    expect(audioConstructorSpy).toHaveBeenCalledWith('/assets/sounds/tap.ogg');
    expect(playSpy).toHaveBeenCalled();

    playReadySound();
    expect(audioConstructorSpy).toHaveBeenCalledWith('/assets/sounds/ready.ogg');

    playGameOverSound();
    expect(audioConstructorSpy).toHaveBeenCalledWith('/assets/sounds/gameover.ogg');

    playCountdownTick();
    expect(audioConstructorSpy).toHaveBeenCalledWith('/assets/sounds/countdown.ogg');

    expect(audioConstructorSpy).toHaveBeenCalledTimes(4);
    expect(playSpy).toHaveBeenCalledTimes(4);
  });

  it('静音时 4 个音效函数不调用 new Audio', () => {
    setMuted(true);
    expect(isMuted()).toBe(true);

    playTapSound();
    playReadySound();
    playGameOverSound();
    playCountdownTick();

    expect(audioConstructorSpy).not.toHaveBeenCalled();
    expect(playSpy).not.toHaveBeenCalled();
  });
});
