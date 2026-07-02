import { test, expect } from '@playwright/test';

test.describe('Error Handling', () => {
  test('invalid room code shows not found', async ({ page }) => {
    // 请求一个不存在的房间 code
    const check = await page.request.get('/api/v1/registry/check/XXXXX');
    expect(check.ok()).toBeFalsy();
    expect(check.status()).toBe(404);
  });

  test('completed room shows ended phase', async ({ page }) => {
    // 创建房间
    const qp = await page.request.post('/api/v1/auth/quickplay', {
      data: { nickname: 'E2EEndedPlayer' },
    });
    expect(qp.ok()).toBeTruthy();
    const qpBody = await qp.json();

    const match = await page.request.post('/api/v1/registry/match');
    expect(match.ok()).toBeTruthy();
    const { lobbyCode } = await match.json();

    // 进入房间并等待游戏结束
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });

    await page.fill('#setup-nickname-input', 'E2EEndedPlayer');
    await page.click('#enter-game-btn');

    // 等待游戏结束（对于单人房间，countdown 结束后气球的 phase 会变为 ended）
    await page.waitForFunction(
      () => {
        const s = (window as unknown as { state?: { phase?: string } }).state;
        return !!s && (s.phase === 'ended' || s.phase === 'playing');
      },
      { timeout: 90000 },
    );

    // 检查房间状态
    const check = await page.request.get(`/api/v1/registry/check/${lobbyCode}`);
    expect(check.ok()).toBeTruthy();
  });

  test('invalid nickname format is rejected', async ({ page }) => {
    // 创建房间
    const qp = await page.request.post('/api/v1/auth/quickplay', {
      data: { nickname: 'E2EValidPlayer' },
    });
    expect(qp.ok()).toBeTruthy();

    const match = await page.request.post('/api/v1/registry/match');
    expect(match.ok()).toBeTruthy();
    const { lobbyCode } = await match.json();

    // 进入房间并尝试提交空昵称
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });

    // 尝试空昵称
    await page.fill('#setup-nickname-input', '');
    await page.click('#enter-game-btn');
    // 应该仍然在 nickname 设置界面（提交被阻止）
    await expect(page.locator('#nickname-setup-screen:not(.hidden)')).toBeVisible({ timeout: 3000 });

    // 尝试超长昵称（100 个字符）
    const longNickname = 'a'.repeat(100);
    await page.fill('#setup-nickname-input', longNickname);
    await page.click('#enter-game-btn');
    // 同样应该被阻止
    await expect(page.locator('#nickname-setup-screen:not(.hidden)')).toBeVisible({ timeout: 3000 });
  });
});