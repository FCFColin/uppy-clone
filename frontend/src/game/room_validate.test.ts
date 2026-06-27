import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getLobbyCodeFromUrl, validateRoomCode, roomErrorMessage } from './room_validate.js';

describe('getLobbyCodeFromUrl', () => {
  it('reads code query param', () => {
    vi.stubGlobal('location', { search: '?code=ROOM123' });
    expect(getLobbyCodeFromUrl()).toBe('ROOM123');
    vi.unstubAllGlobals();
  });
});

describe('validateRoomCode', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  it('returns not_found for 404', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('', { status: 404 }));
    await expect(validateRoomCode('X')).resolves.toEqual({ ok: false, reason: 'not_found' });
    vi.unstubAllGlobals();
  });

  it('returns ended when phase is ended', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      new Response(JSON.stringify({ phase: 'ended' }), { status: 200 }),
    );
    await expect(validateRoomCode('X')).resolves.toEqual({ ok: false, reason: 'ended' });
    vi.unstubAllGlobals();
  });
});

describe('roomErrorMessage', () => {
  it('maps ended rooms', () => {
    expect(roomErrorMessage('ended')).toContain('结束');
  });
});
