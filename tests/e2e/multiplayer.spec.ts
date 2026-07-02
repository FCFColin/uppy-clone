import { test, expect, Page } from '@playwright/test';
import { createRoom, quickplayAuth, matchRoom, connectToRoom, waitForPhase, submitNickname } from './helpers.js';

test.describe('Multiplayer', () => {
  test('two players join same room and see each other', async ({ browser }) => {
    // 创建玩家1的上下文
    const ctx1 = await browser.newContext();
    const page1 = await ctx1.newPage();
    await quickplayAuth(page1);
    const lobbyCode = await matchRoom(page1);

    // 玩家1进入房间
    await page1.goto(`/play.html?code=${lobbyCode}`);
    await expect(page1.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page1, 'Player1');

    // 创建玩家2的上下文
    const ctx2 = await browser.newContext();
    const page2 = await ctx2.newPage();
    await page2.request.post('/api/v1/auth/quickplay', { data: { nickname: 'Player2' } });

    // 玩家2通过相同的 code 匹配到同一房间
    const match2 = await page2.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    expect(match2.ok()).toBeTruthy();
    await page2.goto(`/play.html?code=${lobbyCode}`);
    await expect(page2.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page2, 'Player2');

    // 验证两个玩家都能看到对方
    await expect(page1.locator('#player-list')).toContainText('Player2', { timeout: 10000 });
    await expect(page2.locator('#player-list')).toContainText('Player1', { timeout: 10000 });

    await ctx1.close();
    await ctx2.close();
  });

  test('game counts down and starts for all players', async ({ browser }) => {
    const ctx1 = await browser.newContext();
    const ctx2 = await browser.newContext();
    const page1 = await ctx1.newPage();
    const page2 = await ctx2.newPage();

    // 玩家1创建房间
    await quickplayAuth(page1);
    const lobbyCode = await matchRoom(page1);
    await page1.goto(`/play.html?code=${lobbyCode}`);
    await expect(page1.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page1, 'Player1');

    // 玩家2加入
    await page2.request.post('/api/v1/auth/quickplay', { data: { nickname: 'Player2' } });
    const match2 = await page2.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    expect(match2.ok()).toBeTruthy();
    await page2.goto(`/play.html?code=${lobbyCode}`);
    await expect(page2.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page2, 'Player2');

    // 等待双方进入 playing 阶段
    await waitForPhase(page1, 'playing', 60000);
    await waitForPhase(page2, 'playing', 60000);

    // 确认两个玩家都能看到游戏画布
    await expect(page1.locator('#game-canvas')).toBeVisible();
    await expect(page2.locator('#game-canvas')).toBeVisible();

    await ctx1.close();
    await ctx2.close();
  });

  test('second player tap updates score independently', async ({ browser }) => {
    const ctx1 = await browser.newContext();
    const ctx2 = await browser.newContext();
    const page1 = await ctx1.newPage();
    const page2 = await ctx2.newPage();

    // 玩家1创建房间
    await quickplayAuth(page1);
    const lobbyCode = await matchRoom(page1);
    await page1.goto(`/play.html?code=${lobbyCode}`);
    await expect(page1.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page1, 'Player1');

    // 玩家2加入
    await page2.request.post('/api/v1/auth/quickplay', { data: { nickname: 'Player2' } });
    const match2 = await page2.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    expect(match2.ok()).toBeTruthy();
    await page2.goto(`/play.html?code=${lobbyCode}`);
    await expect(page2.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page2, 'Player2');

    // 等待游戏开始
    await waitForPhase(page1, 'playing', 60000);
    await waitForPhase(page2, 'playing', 60000);

    // 玩家1点击
    const canvas1 = page1.locator('#game-canvas');
    const box1 = await canvas1.boundingBox();
    expect(box1).not.toBeNull();
    if (box1) {
      await page1.mouse.click(box1.x + box1.width / 2, box1.y + box1.height / 2);
    }

    // 玩家2点击
    const canvas2 = page2.locator('#game-canvas');
    const box2 = await canvas2.boundingBox();
    expect(box2).not.toBeNull();
    if (box2) {
      await page2.mouse.click(box2.x + box2.width / 2, box2.y + box2.height / 2);
    }

    // 验证两个玩家各自有得分
    await page1.waitForFunction(
      () => {
        const s = (window as unknown as { state?: { score?: number } }).state;
        return !!s && s.score != null && s.score > 0;
      },
      { timeout: 15000 },
    );

    await page2.waitForFunction(
      () => {
        const s = (window as unknown as { state?: { score?: number } }).state;
        return !!s && s.score != null && s.score > 0;
      },
      { timeout: 15000 },
    );

    await ctx1.close();
    await ctx2.close();
  });

  test('third player is rejected when room is full', async ({ browser }) => {
    // 对于两个玩家的房间，第三个玩家匹配应失败
    const ctx1 = await browser.newContext();
    const ctx2 = await browser.newContext();
    const ctx3 = await browser.newContext();
    const page1 = await ctx1.newPage();
    const page2 = await ctx2.newPage();
    const page3 = await ctx3.newPage();

    // 玩家1创建房间
    await quickplayAuth(page1);
    const lobbyCode = await matchRoom(page1);

    // 玩家1进入房间
    await page1.goto(`/play.html?code=${lobbyCode}`);
    await expect(page1.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page1, 'Player1');

    // 玩家2加入
    await page2.request.post('/api/v1/auth/quickplay', { data: { nickname: 'Player2' } });
    await page2.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    await page2.goto(`/play.html?code=${lobbyCode}`);
    await expect(page2.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page2, 'Player2');

    // 玩家3尝试加入同一房间
    await page3.request.post('/api/v1/auth/quickplay', { data: { nickname: 'Player3' } });
    const match3 = await page3.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    // 房间已满，匹配应该失败
    expect(match3.ok()).toBeFalsy();

    await ctx1.close();
    await ctx2.close();
    await ctx3.close();
  });
});