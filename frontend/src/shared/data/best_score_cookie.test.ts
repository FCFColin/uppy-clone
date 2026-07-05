import { describe, it, expect, beforeEach, vi } from 'vitest';
import { getCookieBestScore, setCookieBestScore, fetchUserBestScore, updateBestScore } from './best_score_cookie.js';

describe('bestScoreCookie', () => {
  beforeEach(() => {
    document.cookie = 'uppy-best-score=; max-age=0; path=/';
  });

  it('returns 0 when no cookie exists', () => {
    expect(getCookieBestScore()).toBe(0);
  });

  it('reads valid cookie value', () => {
    document.cookie = 'uppy-best-score=42';
    expect(getCookieBestScore()).toBe(42);
  });

  it('returns 0 for malformed cookie value', () => {
    document.cookie = 'uppy-best-score=not-a-number';
    expect(getCookieBestScore()).toBe(0);
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

  it('fetchUserBestScore falls back to cookie on API failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network'));
    document.cookie = 'uppy-best-score=99';
    const score = await fetchUserBestScore();
    expect(score).toBe(99);
  });

  it('fetchUserBestScore returns API value on success', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ bestScore: 200 }),
    } as Response);
    const score = await fetchUserBestScore();
    expect(score).toBe(200);
  });

  it('fetchUserBestScore returns 0 when API omits bestScore', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    } as Response);
    const score = await fetchUserBestScore();
    expect(score).toBe(0);
  });
});
