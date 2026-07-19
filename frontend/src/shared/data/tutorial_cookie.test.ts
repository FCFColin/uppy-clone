import { describe, it, expect, beforeEach, vi } from 'vitest';
import { isTutorialDone, markTutorialDone, shouldShowTutorial } from './cookies.js';

describe('tutorialCookie', () => {
  beforeEach(() => {
    document.cookie = 'uppy-tutorial=; max-age=0; path=/';
  });

  it('isTutorialDone returns false initially, true after marking done, sets cookie', () => {
    expect(isTutorialDone()).toBe(false);
    markTutorialDone();
    expect(isTutorialDone()).toBe(true);
    expect(document.cookie).toContain('uppy-tutorial=done');
  });

  it.each([
    ['cookie done', true, null, false],
    ['not done and API fails', false, 'reject', true],
    ['API reports hasHistory', false, { hasHistory: true }, false],
    ['API reports no history', false, { hasHistory: false }, true],
  ] as const)('shouldShowTutorial returns %s when %s', async (_label, cookieDone, apiResponse, expected) => {
    if (cookieDone) markTutorialDone();
    if (apiResponse === 'reject') {
      vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network'));
    } else if (apiResponse) {
      vi.spyOn(globalThis, 'fetch').mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(apiResponse),
      } as Response);
    }
    const result = await shouldShowTutorial();
    expect(result).toBe(expected);
  });
});
