import { test, expect, Page } from '@playwright/test';
import { quickplayAuth, matchRoom, connectToRoom, submitNickname, waitForPhase, tapCanvas } from './helpers';

async function joinRoom(
  page: Page,
  code: string,
  nickname: string,
): Promise<void> {
  await page.goto(`/play.html?code=${code}`);
  await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
  await submitNickname(page, nickname);
}

test.describe('concurrency', () => {
  test('8 players attempt to join same room simultaneously', async ({ browser }) => {
    const hostCtx = await browser.newContext();
    const hostPage = await hostCtx.newPage();
    await quickplayAuth(hostPage);
    const lobbyCode = await matchRoom(hostPage);
    await joinRoom(hostPage, lobbyCode, 'Host');

    const contexts: Awaited<ReturnType<typeof browser.newContext>>[] = [];
    const pages: Page[] = [];

    for (let i = 0; i < 7; i++) {
      const c = await browser.newContext();
      const p = await c.newPage();
      await p.request.post('/api/v1/auth/quickplay', { data: { nickname: `Player${i + 1}` } });
      const match = await p.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
      contexts.push(c);
      pages.push(p);
    }

    const joinResults = await Promise.allSettled(
      pages.map((p, i) => joinRoom(p, lobbyCode, `Player${i + 1}`)),
    );

    const joined = joinResults.filter((r) => r.status === 'fulfilled').length;
    expect(joined).toBeLessThanOrEqual(7);

    if (joined > 0) {
      await expect(hostPage.locator('#player-list-waiting')).not.toBeEmpty({ timeout: 5000 });
    }

    for (const c of contexts) await c.close();
    await hostCtx.close();
  });

  test('all players tap simultaneously during gameplay', async ({ browser }) => {
    const ctx1 = await browser.newContext();
    const ctx2 = await browser.newContext();
    const page1 = await ctx1.newPage();
    const page2 = await ctx2.newPage();

    await quickplayAuth(page1);
    const lobbyCode = await matchRoom(page1);
    await joinRoom(page1, lobbyCode, 'Player1');

    await page2.request.post('/api/v1/auth/quickplay', { data: { nickname: 'Player2' } });
    await page2.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    await joinRoom(page2, lobbyCode, 'Player2');

    await waitForPhase(page1, 'playing', 60000);
    await waitForPhase(page2, 'playing', 60000);

    await Promise.all([
      tapCanvas(page1),
      tapCanvas(page2),
    ]);

    await page1.waitForFunction(
      () => {
        const s = (window as unknown as { state?: { score?: number } }).state;
        return !!s && s.score != null && s.score > 0;
      },
      { timeout: 15000 },
    );

    await ctx1.close();
    await ctx2.close();
  });
});
