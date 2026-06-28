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

  it('redirects 127.0.0.1 without explicit port', () => {
    const replace = vi.fn();
    vi.stubGlobal('location', {
      hostname: '127.0.0.1',
      port: '',
      pathname: '/',
      search: '',
      hash: '',
      replace,
    });
    normalizeAuthHost();
    expect(replace).toHaveBeenCalledWith('http://localhost/');
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

  it('returns ok via quickplay and stores player id', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ userId: 'player-42' }), { status: 200 }));
    await expect(establishGameSession()).resolves.toEqual({ ok: true });
    expect(localStorage.getItem('uppy-player-id')).toBe('player-42');
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
});

describe('sessionErrorMessage', () => {
  it('maps network failures', () => {
    expect(sessionErrorMessage({ ok: false, reason: 'network' })).toContain('网络');
  });

  it('maps rate limit and server failures', () => {
    expect(sessionErrorMessage({ ok: false, reason: 'rate_limit' })).toContain('频繁');
    expect(sessionErrorMessage({ ok: false, reason: 'server' })).toContain('认证失败');
  });
});
