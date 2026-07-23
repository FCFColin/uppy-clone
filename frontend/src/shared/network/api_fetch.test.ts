import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const refreshMock = vi.hoisted(() => vi.fn());
vi.mock('./network.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./network.js')>();
  return {
    ...actual,
    refreshAccessToken: refreshMock,
  };
});

import { apiFetch } from './network.js';

describe('apiFetch', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    refreshMock.mockReset();
    vi.stubGlobal('location', { href: '' });
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

  it.each([
    ['defaults to include', undefined, 'include'],
    ['preserves caller-provided', 'omit', 'omit'],
  ] as const)('credentials %s', async (_label, input, expected) => {
    vi.mocked(fetch).mockResolvedValue({ ok: true, status: 200 } as Response);
    await apiFetch('/api/v1/test', input ? { credentials: input } : undefined);
    const init = vi.mocked(fetch).mock.calls[0]![1] as RequestInit;
    expect(init.credentials).toBe(expected);
  });

  it.each([
    [
      'retries once on network failure then succeeds',
      1,
      [
        ['reject', null],
        ['resolve', { ok: true, status: 200 }],
      ],
      2,
      200,
      false,
    ],
    [
      'throws when retries exhausted',
      1,
      [
        ['reject', null],
        ['reject', null],
      ],
      2,
      null,
      true,
    ],
    ['does not retry when retries is 0', 0, [['reject', null]], 1, null, true],
  ] as const)('%s', async (_label, retries, fetchSequence, expectedCalls, expectedStatus, shouldThrow) => {
    const mockFetch = vi.mocked(fetch);
    for (const [mode, response] of fetchSequence) {
      if (mode === 'reject') {
        mockFetch.mockRejectedValueOnce(new Error('network'));
      } else {
        mockFetch.mockResolvedValueOnce(response as Response);
      }
    }
    const promise = apiFetch('/api/v1/test', { retries });
    if (shouldThrow) {
      await expect(promise).rejects.toThrow('network');
    } else {
      const res = await promise;
      expect(res.status).toBe(expectedStatus);
    }
    expect(fetch).toHaveBeenCalledTimes(expectedCalls);
  });

  it('refreshes token on 401 and retries successfully', async () => {
    let apiCallCount = 0;
    vi.mocked(fetch).mockImplementation((url) => {
      const u = String(url);
      if (u.includes('/auth/refresh')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ refreshed: true }),
        } as Response);
      }
      apiCallCount++;
      if (apiCallCount === 1) {
        return Promise.resolve({ ok: false, status: 401 } as Response);
      }
      return Promise.resolve({ ok: true, status: 200 } as Response);
    });
    const res = await apiFetch('/api/v1/test');
    expect(res.status).toBe(200);
    expect(fetch).toHaveBeenCalledTimes(3);
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
    let refreshCallCount = 0;
    vi.mocked(fetch).mockImplementation((url) => {
      const u = String(url);
      if (u.includes('/auth/refresh')) {
        refreshCallCount++;
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ refreshed: true }),
        } as Response);
      }
      return Promise.resolve({ ok: false, status: 401 } as Response);
    });
    await apiFetch('/api/v1/test', { retries: 0 });
    expect(refreshCallCount).toBe(1);
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
    await expect(apiFetch('/api/v1/test', { signal: controller.signal, retries: 0 })).rejects.toThrow();
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
});
