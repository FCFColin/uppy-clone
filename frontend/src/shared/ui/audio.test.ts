import { describe, it, expect, vi, beforeEach } from 'vitest';
import { playTapSound, playReadySound, playGameOverSound, playCountdownTick } from './audio.js';

describe('audio', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    delete (window as unknown as Record<string, unknown>).AudioContext;
  });

  it('plays sound via oscillator on mock AudioContext', () => {
    const mockOsc = { connect: vi.fn(), start: vi.fn(), stop: vi.fn(), frequency: { value: 0 }, type: '' };
    const mockGain = { connect: vi.fn(), gain: { value: 0 } };
    (window as unknown as Record<string, unknown>).AudioContext = class {
      createOscillator = () => mockOsc;
      createGain = () => mockGain;
      destination = {};
      currentTime = 0;
      resume = vi.fn();
    } as unknown as typeof AudioContext;

    for (const fn of [playTapSound, playReadySound, playGameOverSound, playCountdownTick]) {
      expect(() => fn()).not.toThrow();
    }
    expect(mockOsc.start).toHaveBeenCalled();
  });
});
