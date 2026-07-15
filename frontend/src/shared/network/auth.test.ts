import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  refreshAccessToken,
  logout,
} from './auth.js';

describe('auth token refresh', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it('refreshAccessToken returns false when refresh endpoint fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 401 }));
    expect(await refreshAccessToken()).toBe(false);
  });

  it('refreshAccessToken succeeds via HttpOnly cookie flow', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ refreshed: true }),
    }));
    expect(await refreshAccessToken()).toBe(true);
  });

  it('refreshAccessToken deduplicates concurrent refresh calls', async () => {
    let resolveRefresh: (value: Response) => void = () => {};
    const fetchMock = vi.fn().mockImplementation(
      () => new Promise<Response>((resolve) => { resolveRefresh = resolve; }),
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

  it('refreshAccessToken returns false on network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    expect(await refreshAccessToken()).toBe(false);
  });

  it('logout redirects to home', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
    Object.defineProperty(window, 'location', { value: { href: '' }, writable: true });
    await logout();
    expect(window.location.href).toBe('/');
  });
});
