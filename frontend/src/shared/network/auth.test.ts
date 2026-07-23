import { describe, it, expect, vi, beforeEach } from 'vitest';
import { refreshAccessToken, logout } from './network.js';

describe('auth token refresh', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it.each([
    ['returns false when refresh endpoint fails', { ok: false, status: 401 }, null, false],
    ['succeeds via HttpOnly cookie flow', { ok: true, jsonBody: { refreshed: true } }, null, true],
    ['returns false on network error', null, 'reject', false],
  ] as const)('%s', async (_label, response, mode, expected) => {
    if (mode === 'reject') {
      vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    } else if (response) {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValue({
          ok: response.ok,
          json: async () => (response as Record<string, unknown>).jsonBody,
        }),
      );
    }
    expect(await refreshAccessToken()).toBe(expected);
  });

  it('refreshAccessToken deduplicates concurrent refresh calls', async () => {
    let resolveRefresh: (value: Response) => void = () => {};
    const fetchMock = vi.fn().mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          resolveRefresh = resolve;
        }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const first = refreshAccessToken();
    const second = refreshAccessToken();
    resolveRefresh({
      ok: true,
      json: async () => ({ refreshed: true }),
    } as Response);

    expect(await first).toBe(true);
    expect(await second).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('logout redirects to home', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
    vi.stubGlobal('location', { href: '' });
    await logout();
    expect(window.location.href).toBe('/');
  });
});
