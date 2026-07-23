import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const mockFetchLeaderboard = vi.hoisted(() => vi.fn());
vi.mock('./shared/ui/ui.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./shared/ui/ui.js')>();
  return {
    ...actual,
    fetchLeaderboard: mockFetchLeaderboard,
  };
});

import { renderLeaderboard } from './leaderboard/shared.js';

function flushAsync(): Promise<void> {
  return new Promise((r) => setTimeout(r, 10));
}

describe('renderLeaderboard shared module', () => {
  beforeEach(() => {
    mockFetchLeaderboard.mockReset();
    mockFetchLeaderboard.mockResolvedValue([]);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders tabs, list container, and back-to-hall footer when showBackToLobby=true', async () => {
    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: true });
    await flushAsync();

    expect(container.querySelector('.lb-tabs')).not.toBeNull();
    expect(container.querySelector('.btn-tab.active')?.textContent).toBe('总榜');
    expect(container.querySelector('.lb-full-list')).not.toBeNull();
    // Footer renders a back-to-hall link pointing at the lobby.
    const backToHall = container.querySelector<HTMLAnchorElement>('.lb-footer-actions a.lb-action-btn');
    expect(backToHall).not.toBeNull();
    expect(backToHall!.getAttribute('href')).toBe('/index.html');
    expect(backToHall!.querySelector('.btn-text')?.textContent).toBe('返回大厅');

    container.remove();
  });

  it('omits the back-to-hall footer when showBackToLobby=false (overlay scenario)', async () => {
    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: false, onClose: () => {} });
    await flushAsync();

    expect(container.querySelector('.lb-tabs')).not.toBeNull();
    expect(container.querySelector('.lb-footer-actions a.lb-action-btn')).toBeNull();

    container.remove();
  });

  it('fetches global scope on mount and renders returned entries', async () => {
    mockFetchLeaderboard.mockResolvedValueOnce([
      { rank: 1, name: 'Alice', score: 1000 },
      { rank: 2, name: 'Bob', score: 800 },
    ]);

    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: false });
    await flushAsync();

    expect(mockFetchLeaderboard).toHaveBeenCalledWith('global', 50);
    const items = container.querySelectorAll('.lb-full-list .lb-item');
    expect(items.length).toBe(2);
    expect(items[0]!.classList.contains('lb-rank-1')).toBe(true);
    expect(items[0]!.querySelector('.lb-name')?.textContent).toBe('Alice');
    expect(items[0]!.querySelector('.lb-score')?.textContent).toBe('1,000');
    // Empty/error hints stay hidden when entries render.
    expect(container.querySelector('.lb-full-list')?.nextElementSibling?.classList.contains('hidden')).toBe(true);

    container.remove();
  });

  it('shows the empty hint when the leaderboard returns no entries', async () => {
    mockFetchLeaderboard.mockResolvedValueOnce([]);

    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: false });
    await flushAsync();

    const emptyHint = container.querySelector('.hint:not(.hidden)');
    expect(emptyHint).not.toBeNull();
    expect(emptyHint?.textContent).toBe('暂无记录，快去开一局吧！');

    container.remove();
  });

  it('shows the error hint when fetchLeaderboard throws', async () => {
    mockFetchLeaderboard.mockRejectedValueOnce(new Error('network down'));

    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: false });
    await flushAsync();

    const errorHint = container.querySelector('.hint:not(.hidden)');
    expect(errorHint).not.toBeNull();
    expect(errorHint?.textContent).toBe('加载失败，请确认后端已启动并刷新页面');

    container.remove();
  });

  it('switches to weekly scope and re-fetches when the weekly tab is clicked', async () => {
    mockFetchLeaderboard.mockResolvedValue([]);
    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: false });
    await flushAsync();

    mockFetchLeaderboard.mockClear();
    const tabWeekly = container.querySelectorAll<HTMLButtonElement>('.btn-tab')[1]!;
    expect(tabWeekly.textContent).toBe('周榜');
    tabWeekly.click();
    await flushAsync();

    expect(mockFetchLeaderboard).toHaveBeenCalledWith('weekly', 50);
    expect(tabWeekly.classList.contains('active')).toBe(true);

    container.remove();
  });

  it('re-renders fresh DOM on each call (safe to re-mount on overlay reopen)', async () => {
    mockFetchLeaderboard.mockResolvedValue([]);
    const container = document.createElement('div');
    document.body.append(container);

    await renderLeaderboard(container, { showBackToLobby: false });
    await flushAsync();
    const firstTabs = container.querySelector('.lb-tabs');
    expect(firstTabs).not.toBeNull();

    // Second call (e.g. overlay reopened) should rebuild, not append duplicates.
    await renderLeaderboard(container, { showBackToLobby: false });
    await flushAsync();

    expect(container.querySelectorAll('.lb-tabs').length).toBe(1);

    container.remove();
  });
});
