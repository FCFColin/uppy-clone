import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fetchWithRetry } from './fetch.js';

describe('fetchWithRetry', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('returns response on first success', async () => {
    const response = new Response('ok', { status: 200 });
    vi.mocked(fetch).mockResolvedValueOnce(response);
    await expect(fetchWithRetry('/api/test', {})).resolves.toBe(response);
  });

  it('retries after failure then succeeds', async () => {
    const response = new Response('ok', { status: 200 });
    vi.mocked(fetch)
      .mockRejectedValueOnce(new Error('abort'))
      .mockResolvedValueOnce(response);
    const pending = fetchWithRetry('/api/test', {}, 1);
    await vi.advanceTimersByTimeAsync(500);
    await expect(pending).resolves.toBe(response);
  });

  it('throws after exhausting retries', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('offline'));
    const pending = fetchWithRetry('/api/test', {}, 1);
    const assertion = expect(pending).rejects.toThrow('offline');
    await vi.advanceTimersByTimeAsync(500);
    await assertion;
  });

  it('does not retry when retries is zero', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('fail'));
    await expect(fetchWithRetry('/api/test', {}, 0)).rejects.toThrow('fail');
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1);
  });

  it('returns response after multiple retries', async () => {
    const response = new Response('ok', { status: 200 });
    vi.mocked(fetch)
      .mockRejectedValueOnce(new Error('fail1'))
      .mockRejectedValueOnce(new Error('fail2'))
      .mockResolvedValueOnce(response);
    const pending = fetchWithRetry('/api/test', {}, 2);
    await vi.advanceTimersByTimeAsync(1000);
    await expect(pending).resolves.toBe(response);
  });
});
