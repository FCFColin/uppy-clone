import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  resolveLobbyCode,
  getLobbyCodeFromUrl,
  validateRoomCode,
  roomErrorMessage,
  matchNewRoomCode,
} from './lobby_match.js';

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

describe('getLobbyCodeFromUrl', () => {
  it('reads code query param', () => {
    vi.stubGlobal('location', { search: '?code=ROOMX' });
    expect(getLobbyCodeFromUrl()).toBe('ROOMX');
    vi.unstubAllGlobals();
  });

  it('returns null when code param fails regex', () => {
    vi.stubGlobal('location', { search: '?code=BAD' });
    expect(getLobbyCodeFromUrl()).toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when no code param', () => {
    vi.stubGlobal('location', { search: '' });
    expect(getLobbyCodeFromUrl()).toBeNull();
    vi.unstubAllGlobals();
  });
});

describe('validateRoomCode', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  it('returns not_found for 404', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 404 }));
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: false, reason: 'not_found' });
    vi.unstubAllGlobals();
  });

  it('returns ended when phase is ended', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ phase: 'ended' }), { status: 200 }),
    );
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: false, reason: 'ended' });
    vi.unstubAllGlobals();
  });

  it('returns degraded when response is not ok', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 503 }));
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: true, degraded: true });
    vi.unstubAllGlobals();
  });

  it('returns degraded when API marks degraded', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ degraded: true }), { status: 200 }),
    );
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: true, degraded: true });
    vi.unstubAllGlobals();
  });

  it('returns degraded on network error', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('offline'));
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: true, degraded: true });
    vi.unstubAllGlobals();
  });

  it('returns ok for active room', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ phase: 'waiting' }), { status: 200 }),
    );
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: true });
    vi.unstubAllGlobals();
  });

  it('returns not_found for invalid code format without API call', async () => {
    await expect(validateRoomCode('BAD')).resolves.toEqual({ ok: false, reason: 'not_found' });
    expect(fetch).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('returns not_found for code containing excluded chars', async () => {
    await expect(validateRoomCode('AB01O')).resolves.toEqual({ ok: false, reason: 'not_found' });
    expect(fetch).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});

describe('roomErrorMessage', () => {
  it('maps ended rooms', () => {
    expect(roomErrorMessage('ended')).toContain('结束');
  });

  it('maps missing rooms', () => {
    expect(roomErrorMessage('not_found')).toContain('不存在');
  });
});

describe('matchNewRoomCode', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  it('returns lobby code on success', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ lobbyCode: 'NEW1' }), { status: 200 }),
    );
    await expect(matchNewRoomCode()).resolves.toBe('NEW1');
    vi.unstubAllGlobals();
  });

  it('retries after refresh on 401', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ lobbyCode: 'AUTH' }), { status: 200 }));
    await expect(matchNewRoomCode()).resolves.toBe('AUTH');
    vi.unstubAllGlobals();
  });

  it('returns null when refresh succeeds but lobbyCode missing', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({}), { status: 200 }),
    );
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when match fails after refresh on 401', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response('', { status: 500 }));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when match response is not ok', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 503 }));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when token refresh fails on 401', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when network throws', async () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(fetch).mockRejectedValueOnce(new Error('network error'));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    expect(errSpy).toHaveBeenCalled();
    errSpy.mockRestore();
    vi.unstubAllGlobals();
  });
});