import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
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
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ lobbyCode: 'MATCH' }),
        clone() {
          return this;
        },
      }),
    );
    await expect(resolveLobbyCode()).resolves.toBe('MATCH');
  });

  // Adversarial: 401 triggers refresh retry before giving up.
  it('retries match after token refresh on 401', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ refreshed: true }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ lobbyCode: 'RETRY' }),
        clone() {
          return this;
        },
      });
    vi.stubGlobal('fetch', fetchMock);
    await expect(resolveLobbyCode()).resolves.toBe('RETRY');
  });

  it.each([
    ['network error', vi.fn().mockRejectedValue(new Error('network down')), null],
    ['non-401 failure', vi.fn().mockResolvedValue({ ok: false, status: 500 }), null],
    [
      'refresh failure after 401',
      vi.fn().mockResolvedValueOnce({ ok: false, status: 401 }).mockResolvedValueOnce({ ok: false, status: 401 }),
      null,
    ],
    [
      'retry failure after refresh',
      vi
        .fn()
        .mockResolvedValueOnce({ ok: false, status: 401 })
        .mockResolvedValueOnce({ ok: true, json: async () => ({ refreshed: true }) })
        .mockResolvedValueOnce({ ok: false, status: 503 }),
      null,
    ],
  ])('returns null when match API fails due to %s', async (_label, fetchMock, _expected) => {
    if (_label === 'network error') {
      vi.spyOn(console, 'error').mockImplementation(() => {});
    }
    vi.stubGlobal('fetch', fetchMock);
    await expect(resolveLobbyCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });
});

describe('getLobbyCodeFromUrl', () => {
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    ['?code=ROOMX', 'ROOMX'],
    ['?code=BAD', null],
    ['', null],
  ])('search %s returns %s', (search, expected) => {
    vi.stubGlobal('location', { search });
    expect(getLobbyCodeFromUrl()).toBe(expected);
  });
});

describe('validateRoomCode', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => vi.unstubAllGlobals());

  it.each([
    [404, null, { ok: false, reason: 'not_found' }],
    [200, { phase: 'ended' }, { ok: true }],
    [503, null, { ok: true, degraded: true }],
    [200, { degraded: true }, { ok: true, degraded: true }],
    [200, { phase: 'waiting' }, { ok: true }],
  ])('returns %s for status %s body %s', async (status, body, expected) => {
    vi.mocked(fetch).mockResolvedValueOnce(
      body ? new Response(JSON.stringify(body), { status }) : new Response('', { status }),
    );
    await expect(validateRoomCode('ABCDE')).resolves.toEqual(expected);
  });

  it('returns degraded on network error', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('offline'));
    await expect(validateRoomCode('ABCDE')).resolves.toEqual({ ok: true, degraded: true });
  });

  it.each([
    ['BAD', /not.*found/],
    ['AB01O', /not.*found/],
  ])('returns not_found for invalid code %s without API call', async (code) => {
    await expect(validateRoomCode(code)).resolves.toEqual({ ok: false, reason: 'not_found' });
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe('roomErrorMessage', () => {
  it.each([
    ['not_found', '不存在'],
  ])('maps %s rooms', (reason, expected) => {
    expect(roomErrorMessage(reason as 'not_found')).toContain(expected);
  });
});

describe('matchNewRoomCode', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => vi.unstubAllGlobals());

  it('returns lobby code on success, retries after refresh on 401', async () => {
    // Direct success
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ lobbyCode: 'NEW1' }), { status: 200 }));
    await expect(matchNewRoomCode()).resolves.toBe('NEW1');

    // Refresh + retry success
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ lobbyCode: 'AUTH' }), { status: 200 }));
    await expect(matchNewRoomCode()).resolves.toBe('AUTH');
  });

  it.each([
    ['refresh succeeds but lobbyCode missing', [new Response(JSON.stringify({}), { status: 200 })], false],
    [
      'match fails after refresh on 401',
      [
        new Response('', { status: 401 }),
        new Response(JSON.stringify({ refreshed: true }), { status: 200 }),
        new Response('', { status: 500 }),
      ],
      false,
    ],
    ['match response is not ok', [new Response('', { status: 503 })], false],
    [
      'token refresh fails on 401',
      [new Response('', { status: 401 }), new Response(JSON.stringify({ refreshed: false }), { status: 401 })],
      false,
    ],
    ['network throws', [], true],
  ])('returns null when %s', async (_label, responses, useReject) => {
    const errSpy = useReject ? vi.spyOn(console, 'error').mockImplementation(() => {}) : null;
    if (useReject) {
      vi.mocked(fetch).mockRejectedValueOnce(new Error('network error'));
    } else {
      for (const res of responses) {
        vi.mocked(fetch).mockResolvedValueOnce(res);
      }
    }
    await expect(matchNewRoomCode()).resolves.toBeNull();
    if (errSpy) {
      expect(errSpy).toHaveBeenCalled();
      errSpy.mockRestore();
    }
  });
});
