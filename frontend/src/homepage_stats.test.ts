import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock api_fetch. vi.mock is hoisted, so the mock applies to dynamic imports
// performed inside beforeEach as well.
vi.mock('./shared/network/api_fetch.js', () => ({
  apiFetch: vi.fn(),
}));

function setDom(): void {
  document.body.innerHTML = `
    <span id="stat-online-value">--</span>
    <span id="stat-games-value">--</span>
    <span id="stat-best-value">--</span>
    <span id="join-count">--</span>
  `;
}

describe('initHomepageStats', () => {
  let initHomepageStats: () => void;
  let apiFetch: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    // Reset module registry so homepage_stats.ts re-executes and its
    // module-level `timer` variable is re-initialized to null on every test.
    // Without this, the first test that calls initHomepageStats() flips
    // `timer` to non-null and subsequent tests' start() early-returns,
    // skipping setInterval registration.
    vi.resetModules();
    // Explicitly list timers to fake — vitest 4's default toFake set
    // does not always include setInterval in this environment.
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'],
    });
    setDom();
    // jsdom defines `hidden` as a getter on Document.prototype and defaults
    // to true. The setInterval callback inside start() early-returns when
    // hidden is true, so mock the getter to false for polling tests.
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);

    const mod = await import('./homepage_stats.js');
    initHomepageStats = mod.initHomepageStats;
    const apiMod = await import('./shared/network/api_fetch.js');
    apiFetch = apiMod.apiFetch as ReturnType<typeof vi.fn>;
    apiFetch.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('updates DOM on successful fetch', async () => {
    apiFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ onlinePlayers: 10, gamesToday: 5, bestScore: 100, activeRooms: 2 }),
    });
    initHomepageStats();
    await vi.advanceTimersByTimeAsync(0);
    expect(document.getElementById('stat-online-value')?.textContent).toBe('10');
    expect(document.getElementById('stat-games-value')?.textContent).toBe('5');
    expect(document.getElementById('stat-best-value')?.textContent).toBe('100');
    expect(document.getElementById('join-count')?.textContent).toBe('+2');
  });

  it('shows placeholder on fetch failure', async () => {
    apiFetch.mockRejectedValueOnce(new Error('net'));
    initHomepageStats();
    await vi.advanceTimersByTimeAsync(0);
    expect(document.getElementById('stat-online-value')?.textContent).toBe('--');
    expect(document.getElementById('join-count')?.textContent).toBe('--');
  });

  it('polls every 30 seconds', async () => {
    apiFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ onlinePlayers: 1, gamesToday: 1, bestScore: 1, activeRooms: 1 }),
    });
    initHomepageStats();
    await vi.advanceTimersByTimeAsync(0);
    expect(apiFetch).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(apiFetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(apiFetch).toHaveBeenCalledTimes(3);
  });
});
