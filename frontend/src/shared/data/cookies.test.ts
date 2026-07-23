import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  getCookieBestScore,
  fetchUserBestScore,
  updateBestScore,
  isTutorialDone,
  markTutorialDone,
  shouldShowTutorial,
} from './cookies.js';

describe('bestScoreCookie', () => {
  beforeEach(() => {
    document.cookie = 'uppy-best-score=; max-age=0; path=/';
  });

  it.each([
    ['no cookie', '', 0],
    ['valid value', '42', 42],
    ['malformed value', 'not-a-number', 0],
  ] as const)('getCookieBestScore returns %s', (_label, cookieValue, expected) => {
    if (cookieValue) document.cookie = `uppy-best-score=${cookieValue}`;
    expect(getCookieBestScore()).toBe(expected);
  });

  it('updateBestScore only saves higher score', () => {
    document.cookie = 'uppy-best-score=50';
    const r1 = updateBestScore(30);
    expect(r1).toEqual({ best: 50, isNewRecord: false });
    expect(document.cookie).toContain('uppy-best-score=50');

    const r2 = updateBestScore(70);
    expect(r2).toEqual({ best: 70, isNewRecord: true });
    expect(document.cookie).toContain('uppy-best-score=70');
  });

  it.each([
    ['falls back to cookie on API failure', 'reject', null, 99, 99, 'uppy-best-score=99'],
    ['returns API value on success', 'resolve-ok', { bestScore: 200 }, 0, 200, null],
    ['returns 0 when API omits bestScore', 'resolve-ok', {}, 0, 0, null],
    [
      'writes API score back to cookie when higher than cookie',
      'resolve-ok',
      { bestScore: 200 },
      50,
      200,
      'uppy-best-score=200',
    ],
    [
      'does not overwrite cookie when API score is lower',
      'resolve-ok',
      { bestScore: 100 },
      300,
      100,
      'uppy-best-score=300',
    ],
  ] as const)('fetchUserBestScore %s', async (_label, mode, apiBody, cookieInitial, expectedScore, expectedCookie) => {
    if (cookieInitial) document.cookie = `uppy-best-score=${cookieInitial}`;
    if (mode === 'reject') {
      vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network'));
    } else {
      vi.spyOn(globalThis, 'fetch').mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(apiBody),
      } as Response);
    }
    const score = await fetchUserBestScore();
    expect(score).toBe(expectedScore);
    if (expectedCookie) expect(document.cookie).toContain(expectedCookie);
  });
});

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
