import { describe, it, expect, vi, beforeEach } from 'vitest';

const sessionMocks = vi.hoisted(() => ({
  establishGameSession: vi.fn(async () => ({ ok: true })),
}));

vi.mock('../shared/session.js', () => ({
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

  it('returns false when establishGameSession fails', async () => {
    sessionMocks.establishGameSession.mockResolvedValue({ ok: false, reason: 'server' });
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(ensureAuth()).resolves.toBe(false);
    err.mockRestore();
  });
});
