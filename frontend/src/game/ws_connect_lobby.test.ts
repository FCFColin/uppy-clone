import { describe, it, expect, vi, beforeEach } from 'vitest';
import { resolveLobbyCode } from './ws_connect_lobby.js';
import { storeRefreshToken } from '../shared/auth.js';

describe('resolveLobbyCode', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    storeRefreshToken('');
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
    storeRefreshToken('rt');
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ refresh_token: 'rt2' }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ lobbyCode: 'RETRY' }) });
    vi.stubGlobal('fetch', fetchMock);
    await expect(resolveLobbyCode()).resolves.toBe('RETRY');
  });
});
