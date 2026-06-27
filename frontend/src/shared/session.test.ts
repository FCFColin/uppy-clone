import { describe, it, expect, vi, beforeEach } from 'vitest';
import { normalizeAuthHost, establishGameSession, sessionErrorMessage } from './session.js';

describe('normalizeAuthHost', () => {
  it('redirects 127.0.0.1 to localhost', () => {
    const replace = vi.fn();
    vi.stubGlobal('location', {
      hostname: '127.0.0.1',
      port: '5173',
      pathname: '/play.html',
      search: '?code=ABC',
      hash: '',
      replace,
    });
    normalizeAuthHost();
    expect(replace).toHaveBeenCalledWith('http://localhost:5173/play.html?code=ABC');
    vi.unstubAllGlobals();
  });
});

describe('establishGameSession', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    localStorage.clear();
  });

  it('returns ok when auth check succeeds', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
  });

  // Adversarial: rate limit on quickplay fallback
  it('returns rate_limit when quickplay returns 429', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response('', { status: 429 }));
    const result = await establishGameSession();
    expect(result).toEqual({ ok: false, status: 429, reason: 'rate_limit' });
    vi.unstubAllGlobals();
  });
});

describe('sessionErrorMessage', () => {
  it('maps network failures', () => {
    expect(sessionErrorMessage({ ok: false, reason: 'network' })).toContain('网络');
  });
});
