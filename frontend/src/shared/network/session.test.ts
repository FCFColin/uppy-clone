import { describe, it, expect, vi, beforeEach } from 'vitest';
import { normalizeAuthHost, establishGameSession, sessionErrorMessage } from './network.js';

describe('normalizeAuthHost', () => {
  it.each([
    ['127.0.0.1 with port', '5173', '/play.html', '?code=ABC', 'http://localhost:5173/play.html?code=ABC'],
    ['127.0.0.1 without port', '', '/', '', 'http://localhost/'],
  ] as const)('redirects %s to localhost', (_label, port, pathname, search, expectedUrl) => {
    const replace = vi.fn();
    vi.stubGlobal('location', {
      hostname: '127.0.0.1',
      port,
      pathname,
      search,
      hash: '',
      replace,
    });
    normalizeAuthHost();
    expect(replace).toHaveBeenCalledWith(expectedUrl);
    vi.unstubAllGlobals();
  });
});

describe('establishGameSession', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    localStorage.clear();
    sessionStorage.clear();
  });

  it('returns ok when auth check succeeds', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
  });

  it('returns ok after refresh and recheck succeed', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response('', { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
  });

  it('returns ok via quickplay', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ userId: 'player-42' }), { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
  });

  it('falls through to quickplay when refresh succeeds but recheck fails', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response('', { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
    expect(fetch).toHaveBeenCalledTimes(4);
    vi.unstubAllGlobals();
  });

  it('uses quick auth check when session flag is set', async () => {
    sessionStorage.setItem('uppy-auth-ready', '1');
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
    expect(sessionStorage.getItem('uppy-auth-ready')).toBeNull();
  });

  it('falls through when auth-ready quick check fails', async () => {
    sessionStorage.setItem('uppy-auth-ready', '1');
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response('', { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
    vi.unstubAllGlobals();
  });

  it('quickplay sends saved nickname when present', async () => {
    localStorage.setItem('uppy-nickname', 'Ace');
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ userId: 'player-99' }), { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
    expect(JSON.parse(vi.mocked(fetch).mock.calls[2]![1]!.body as string)).toEqual({ nickname: 'Ace' });
    vi.unstubAllGlobals();
  });

  // Adversarial: rate limit on quickplay fallback
  it('returns rate_limit when quickplay returns 429', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }))
      .mockResolvedValueOnce(new Response('', { status: 429 }));
    const result = await establishGameSession();
    expect(result).toEqual({ ok: false, status: 429, reason: 'rate_limit' });
    vi.unstubAllGlobals();
  });

  it('returns network reason when fetch throws', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('offline'));
    await expect(establishGameSession()).resolves.toEqual({ ok: false, reason: 'network' });
    vi.unstubAllGlobals();
  });

  it('returns server reason when quickplay fails with non-429 status', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }))
      .mockResolvedValueOnce(new Response('', { status: 500 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: false, status: 500, reason: 'server' });
    vi.unstubAllGlobals();
  });

  // shared-002: POST is non-idempotent — quickplay must NOT retry on failure.
  it('does not retry quickplay POST on network failure (retries=0)', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }))
      .mockRejectedValueOnce(new Error('network down'));
    const result = await establishGameSession();
    expect(result).toEqual({ ok: false, reason: 'network' });
    // Exactly 3 calls: auth/check, auth/refresh, quickplay POST (no retry).
    expect(fetch).toHaveBeenCalledTimes(3);
    vi.unstubAllGlobals();
  });
});

describe('sessionErrorMessage', () => {
  it.each([
    ['network', '网络'],
    ['rate_limit', '频繁'],
    ['server', '认证失败'],
  ] as const)('maps %s reason to expected message', (reason, expected) => {
    expect(sessionErrorMessage({ ok: false, reason })).toContain(expected);
  });
});
