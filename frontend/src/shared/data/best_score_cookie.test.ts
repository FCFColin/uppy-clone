import { describe, it, expect, beforeEach, vi } from 'vitest';
import { getCookieBestScore, setCookieBestScore, fetchUserBestScore, updateBestScore } from './cookies.js';

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

  it('writes score to cookie', () => {
    setCookieBestScore(100);
    expect(document.cookie).toContain('uppy-best-score=100');
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

  it('updateBestScore marks new record when no prior cookie', () => {
    const r = updateBestScore(10);
    expect(r).toEqual({ best: 10, isNewRecord: true });
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
