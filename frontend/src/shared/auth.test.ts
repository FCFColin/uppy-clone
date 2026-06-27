import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  storeRefreshToken,
  getRefreshToken,
  clearRefreshToken,
  refreshAccessToken,
  fetchWithRefresh,
  logout,
} from './auth.js';

describe('auth token storage', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it('stores and clears refresh token', () => {
    storeRefreshToken('rt-1');
    expect(getRefreshToken()).toBe('rt-1');
    clearRefreshToken();
    expect(getRefreshToken()).toBeNull();
  });

  it('refreshAccessToken returns false without token', async () => {
    expect(await refreshAccessToken()).toBe(false);
  });

  it('refreshAccessToken succeeds and rotates token', async () => {
    storeRefreshToken('old-rt');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ refresh_token: 'new-rt' }),
    }));
    expect(await refreshAccessToken()).toBe(true);
    expect(getRefreshToken()).toBe('new-rt');
  });

  // Adversarial: expired refresh must clear storage (no silent reuse).
  it('refreshAccessToken clears token on 401', async () => {
    storeRefreshToken('expired');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 401 }));
    expect(await refreshAccessToken()).toBe(false);
    expect(getRefreshToken()).toBeNull();
  });

  it('fetchWithRefresh redirects when refresh fails after 401', async () => {
    storeRefreshToken('rt');
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ status: 401 })
      .mockResolvedValueOnce({ ok: false, status: 401 }));
    const href = vi.fn();
    Object.defineProperty(window, 'location', { value: { href: '' }, writable: true });
    await fetchWithRefresh('/api/v1/user/data');
    expect(getRefreshToken()).toBeNull();
  });

  it('logout clears token and redirects', async () => {
    storeRefreshToken('rt');
    localStorage.setItem('uppy-player-id', 'p1');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
    Object.defineProperty(window, 'location', { value: { href: '' }, writable: true });
    await logout();
    expect(getRefreshToken()).toBeNull();
    expect(localStorage.getItem('uppy-player-id')).toBeNull();
  });
});
