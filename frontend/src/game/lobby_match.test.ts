import { describe, it, expect, vi, beforeEach } from 'vitest';
import { resolveLobbyCode } from './lobby_match.js';

describe('resolveLobbyCode', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    history.replaceState({}, '', '/');
  });

  it('returns code from URL query param', async () => {
    history.replaceState({}, '', '/?code=ABCD2');
    await expect(resolveLobbyCode()).resolves.toBe('ABCD2');
  });

  it('calls match API when no URL code', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ lobbyCode: 'MATCH' }),
    }));
    await expect(resolveLobbyCode()).resolves.toBe('MATCH');
  });

  // Adversarial: 401 triggers refresh retry before giving up.
  it('retries match after token refresh on 401', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ refreshed: true }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ lobbyCode: 'RETRY' }) });
    vi.stubGlobal('fetch', fetchMock);
    await expect(resolveLobbyCode()).resolves.toBe('RETRY');
  });

  it('returns null when match API throws', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));
    await expect(resolveLobbyCode()).resolves.toBeNull();
    expect(errSpy).toHaveBeenCalled();
    errSpy.mockRestore();
  });

  it('returns null when match API fails without 401', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500 }));
    await expect(resolveLobbyCode()).resolves.toBeNull();
  });

  it('returns null when refresh fails after 401', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: false, status: 401 }));
    await expect(resolveLobbyCode()).resolves.toBeNull();
  });

  it('returns null when retry after refresh is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ refreshed: true }) })
      .mockResolvedValueOnce({ ok: false, status: 503 }));
    await expect(resolveLobbyCode()).resolves.toBeNull();
  });
});
