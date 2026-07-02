import { describe, it, expect, vi, beforeEach } from 'vitest';

const sessionMocks = vi.hoisted(() => ({
  establishGameSession: vi.fn(async () => ({ ok: true })),
}));

vi.mock('../shared/network/session.js', () => ({
  establishGameSession: sessionMocks.establishGameSession,
}));

import { ensureAuth } from './ws_auth.js';

describe('ensureAuth', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns true when establishGameSession succeeds', async () => {
    sessionMocks.establishGameSession.mockResolvedValue({ ok: true });
    await expect(ensureAuth()).resolves.toBe(true);
  });

  it('returns false when establishGameSession fails with server reason', async () => {
    sessionMocks.establishGameSession.mockResolvedValue({ ok: false, reason: 'server' } as import('../shared/network/session.js').SessionResult);
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(ensureAuth()).resolves.toBe(false);
    err.mockRestore();
  });

  it('returns false on rate limit', async () => {
    sessionMocks.establishGameSession.mockResolvedValue({ ok: false, reason: 'rate_limit' } as import('../shared/network/session.js').SessionResult);
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(ensureAuth()).resolves.toBe(false);
    err.mockRestore();
  });

  it('returns false on network error', async () => {
    sessionMocks.establishGameSession.mockResolvedValue({ ok: false, reason: 'network' } as import('../shared/network/session.js').SessionResult);
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(ensureAuth()).resolves.toBe(false);
    err.mockRestore();
  });
});
