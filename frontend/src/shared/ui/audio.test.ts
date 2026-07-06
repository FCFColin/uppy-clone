import { describe, it, expect, vi, beforeEach } from 'vitest';
import { playTapSound, playReadySound, playGameOverSound, playCountdownTick, vibrate, resumeAudioContext } from './audio.js';

describe('audio', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    delete (window as unknown as Record<string, unknown>).AudioContext;
  });

  it('handles AudioContext creation failure', () => {
    (window as unknown as Record<string, unknown>).AudioContext = class {
      constructor() { throw new Error('not supported'); }
    } as unknown as typeof AudioContext;
    expect(() => playTapSound()).not.toThrow();
    expect(() => playReadySound()).not.toThrow();
    expect(() => playGameOverSound()).not.toThrow();
    expect(() => playCountdownTick()).not.toThrow();
  });

  it('plays sounds with mock AudioContext', () => {
    const mockOsc = { connect: vi.fn(), start: vi.fn(), stop: vi.fn(), frequency: { value: 0 }, type: '' };
    const mockGain = { connect: vi.fn(), gain: { value: 0 } };
    (window as unknown as Record<string, unknown>).AudioContext = class {
      createOscillator = () => mockOsc;
      createGain = () => mockGain;
      destination = {};
      currentTime = 0;
      resume = vi.fn();
    } as unknown as typeof AudioContext;

    expect(() => playTapSound()).not.toThrow();
    expect(mockOsc.start).toHaveBeenCalled();
  });

  it('plays different sound types with mock AudioContext', () => {
    const mockOsc = { connect: vi.fn(), start: vi.fn(), stop: vi.fn(), frequency: { value: 0 }, type: '' };
    const mockGain = { connect: vi.fn(), gain: { value: 0 } };
    (window as unknown as Record<string, unknown>).AudioContext = class {
      createOscillator = () => mockOsc;
      createGain = () => mockGain;
      destination = {};
      currentTime = 0;
      resume = vi.fn();
    } as unknown as typeof AudioContext;

    expect(() => playTapSound()).not.toThrow();
    expect(() => playReadySound()).not.toThrow();
    expect(() => playGameOverSound()).not.toThrow();
    expect(() => playCountdownTick()).not.toThrow();
  });

  it('vibrate does not throw', () => {
    if (typeof navigator.vibrate === 'function') {
      vi.spyOn(navigator, 'vibrate').mockImplementation(() => true);
    }
    expect(() => vibrate(100)).not.toThrow();
    expect(() => vibrate([100, 50, 100])).not.toThrow();
  });

  it('resumeAudioContext does not throw', () => {
    const mockAc = { resume: vi.fn() };
    (window as unknown as Record<string, unknown>).AudioContext = class {
      constructor() { return mockAc; }
    } as unknown as typeof AudioContext;
    expect(() => resumeAudioContext()).not.toThrow();
  });
});
