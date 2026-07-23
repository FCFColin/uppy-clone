import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('./shared/network/network.js', () => ({
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
    vi.resetModules();
    vi.useFakeTimers({
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'],
    });
    setDom();
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);

    const mod = await import('./homepage_stats.js');
    initHomepageStats = mod.initHomepageStats;
    const apiMod = await import('./shared/network/network.js');
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
