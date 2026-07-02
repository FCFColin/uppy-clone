import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getLobbyCodeFromUrl, validateRoomCode, roomErrorMessage } from './room_validate.js';

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
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ lobbyCode: 'NEW1' }), { status: 200 }),
    );
    await expect(matchNewRoomCode()).resolves.toBe('NEW1');
    vi.unstubAllGlobals();
  });

  it('retries after refresh on 401', async () => {
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ lobbyCode: 'AUTH' }), { status: 200 }));
    await expect(matchNewRoomCode()).resolves.toBe('AUTH');
    vi.unstubAllGlobals();
  });

  it('returns null when refresh succeeds but lobbyCode missing', async () => {
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({}), { status: 200 }),
    );
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when match fails after refresh on 401', async () => {
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: true }), { status: 200 }))
      .mockResolvedValueOnce(new Response('', { status: 500 }));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when match response is not ok', async () => {
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 503 }));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when token refresh fails on 401', async () => {
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch)
      .mockResolvedValueOnce(new Response('', { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ refreshed: false }), { status: 401 }));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });

  it('returns null when network throws', async () => {
    const { matchNewRoomCode } = await import('./room_validate.js');
    vi.mocked(fetch).mockRejectedValueOnce(new Error('network error'));
    await expect(matchNewRoomCode()).resolves.toBeNull();
    vi.unstubAllGlobals();
  });
});
