import { describe, it, expect, beforeEach, vi } from 'vitest';
import { isTutorialDone, markTutorialDone, shouldShowTutorial } from './tutorial_cookie.js';

describe('tutorialCookie', () => {
  beforeEach(() => {
    document.cookie = 'uppy-tutorial=; max-age=0; path=/';
  });

  it('isTutorialDone returns false initially', () => {
    expect(isTutorialDone()).toBe(false);
  });

  it('isTutorialDone returns true after marking done', () => {
    markTutorialDone();
    expect(isTutorialDone()).toBe(true);
  });

  it('sets cookie with done value', () => {
    markTutorialDone();
    expect(document.cookie).toContain('uppy-tutorial=done');
  });

  it('shouldShowTutorial returns false when cookie is done', async () => {
    markTutorialDone();
    const result = await shouldShowTutorial();
    expect(result).toBe(false);
  });

  it('shouldShowTutorial returns true when not done and API fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network'));
    const result = await shouldShowTutorial();
    expect(result).toBe(true);
  });

  it('shouldShowTutorial returns false when API reports hasHistory', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ hasHistory: true }),
    } as Response);
    const result = await shouldShowTutorial();
    expect(result).toBe(false);
  });

  it('shouldShowTutorial returns true when API reports no history', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ hasHistory: false }),
    } as Response);
    const result = await shouldShowTutorial();
    expect(result).toBe(true);
  });
});
