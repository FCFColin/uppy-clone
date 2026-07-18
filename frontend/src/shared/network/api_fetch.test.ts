import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock refreshAccessToken so apiFetch's 401-refresh path can be controlled.
const refreshMock = vi.hoisted(() => vi.fn());
vi.mock('./auth.js', () => ({
  refreshAccessToken: refreshMock,
}));

import { apiFetch } from './api_fetch.js';

describe('apiFetch', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    refreshMock.mockReset();
    // Prevent window.location.href mutation from polluting other tests.
    Object.defineProperty(window, 'location', {
      value: { href: '' },
      writable: true,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns response on successful request', async () => {
    const okRes = { ok: true, status: 200 } as Response;
    vi.mocked(fetch).mockResolvedValue(okRes);
    const res = await apiFetch('/api/v1/test');
    expect(res).toBe(okRes);
    expect(fetch).toHaveBeenCalledOnce();
  });

  it('defaults credentials to include', async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: true, status: 200 } as Response);
    await apiFetch('/api/v1/test');
    const init = vi.mocked(fetch).mock.calls[0]![1] as RequestInit;
    expect(init.credentials).toBe('include');
  });

  it('preserves caller-provided credentials', async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: true, status: 200 } as Response);
    await apiFetch('/api/v1/test', { credentials: 'omit' });
    const init = vi.mocked(fetch).mock.calls[0]![1] as RequestInit;
    expect(init.credentials).toBe('omit');
  });

  it('retries once on network failure then succeeds', async () => {
    vi.mocked(fetch)
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({ ok: true, status: 200 } as Response);
    const res = await apiFetch('/api/v1/test', { retries: 1 });
    expect(res.status).toBe(200);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('throws when retries exhausted', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('network'));
    await expect(apiFetch('/api/v1/test', { retries: 1 })).rejects.toThrow('network');
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('does not retry when retries is 0', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('network'));
    await expect(apiFetch('/api/v1/test', { retries: 0 })).rejects.toThrow('network');
    expect(fetch).toHaveBeenCalledOnce();
  });

  it('refreshes token on 401 and retries successfully', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce({ ok: false, status: 401 } as Response)
      .mockResolvedValueOnce({ ok: true, status: 200 } as Response);
    refreshMock.mockResolvedValue(true);
    const res = await apiFetch('/api/v1/test');
    expect(res.status).toBe(200);
    expect(refreshMock).toHaveBeenCalledOnce();
    // First call (401) + retry call (200) = 2 fetch calls, no retry slot consumed.
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('redirects to / when refresh fails on 401', async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: false, status: 401 } as Response);
    refreshMock.mockResolvedValue(false);
    const res = await apiFetch('/api/v1/test');
    expect(res.status).toBe(401);
    expect(window.location.href).toBe('/');
  });

  it('returns 401 without refresh when autoRefresh is false', async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: false, status: 401 } as Response);
    const res = await apiFetch('/api/v1/test', { autoRefresh: false });
    expect(res.status).toBe(401);
    expect(refreshMock).not.toHaveBeenCalled();
  });

  it('does not refresh twice on repeated 401', async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: false, status: 401 } as Response);
    refreshMock.mockResolvedValue(true);
    await apiFetch('/api/v1/test', { retries: 0 });
    // After one successful refresh + one retry that still 401s, refresh is not
    // attempted again (hasRefreshed guard).
    expect(refreshMock).toHaveBeenCalledOnce();
  });

  it('aborts when external signal is already aborted', async () => {
    vi.mocked(fetch).mockImplementation((_url, init) => {
      const signal = (init as RequestInit).signal;
      if (signal?.aborted) {
        return Promise.reject(new DOMException('aborted', 'AbortError'));
      }
      return Promise.resolve({ ok: true, status: 200 } as Response);
    });
    const controller = new AbortController();
    controller.abort();
    await expect(
      apiFetch('/api/v1/test', { signal: controller.signal, retries: 0 }),
    ).rejects.toThrow();
  });

  it('propagates abort when external signal aborts mid-request', async () => {
    vi.mocked(fetch).mockImplementation((_url, init) => {
      const signal = (init as RequestInit).signal;
      return new Promise((_resolve, reject) => {
        signal?.addEventListener('abort', () => {
          reject(new DOMException('aborted', 'AbortError'));
        });
      });
    });
    const controller = new AbortController();
    const promise = apiFetch('/api/v1/test', { signal: controller.signal, retries: 0 });
    controller.abort();
    await expect(promise).rejects.toThrow();
  });

  it('cleans up external signal listener after success', async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: true, status: 200 } as Response);
    const controller = new AbortController();
    const removeSpy = vi.spyOn(controller.signal, 'removeEventListener');
    await apiFetch('/api/v1/test', { signal: controller.signal, retries: 0 });
    expect(removeSpy).toHaveBeenCalled();
  });
});
