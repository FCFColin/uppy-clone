import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  refreshAccessToken,
  fetchWithRefresh,
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

  it('fetchWithRefresh retries original request after successful refresh', async () => {
    const okResponse = { ok: true, status: 200 };
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ refreshed: true }) })
      .mockResolvedValueOnce(okResponse));
    const res = await fetchWithRefresh('/api/v1/user/data');
    expect(res).toBe(okResponse);
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(3);
  });

  it('fetchWithRefresh redirects when refresh fails after 401', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ status: 401 })
      .mockResolvedValueOnce({ ok: false, status: 401 }));
    Object.defineProperty(window, 'location', { value: { href: '' }, writable: true });
    await fetchWithRefresh('/api/v1/user/data');
    expect(window.location.href).toBe('/');
  });

  it('fetchWithRefresh returns response without refresh when request succeeds', async () => {
    const okResponse = { status: 200, ok: true };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse));
    const res = await fetchWithRefresh('/api/v1/user/data');
    expect(res).toBe(okResponse);
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1);
  });

  it('logout redirects to home', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
    Object.defineProperty(window, 'location', { value: { href: '' }, writable: true });
    await logout();
    expect(window.location.href).toBe('/');
  });
});
