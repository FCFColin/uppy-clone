import { test, expect } from '@playwright/test';
import { createRoom, connectToRoom, submitNickname } from './helpers';

test.describe('network boundary conditions', () => {
  test('reconnects after WebSocket disconnect', async ({ page }) => {
    const code = await createRoom(page);
    await connectToRoom(page, code);
    await submitNickname(page, 'player1');

    await page.evaluate(() => {
      const ws = (window as unknown as { __ws?: WebSocket }).__ws;
      if (ws) ws.close();
    });

    await page.waitForTimeout(1000);
    await page.goto(`/play.html?code=${code}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 10000 });
    await page.waitForFunction(
      () => (window as unknown as { state?: { nicknameSubmitted?: boolean } }).state?.nicknameSubmitted === true,
      { timeout: 10000 },
    );
  });

  test('recovers from multiple rapid disconnects', async ({ page }) => {
    const code = await createRoom(page);
    await connectToRoom(page, code);
    await submitNickname(page, 'player1');

    for (let i = 0; i < 5; i++) {
      await page.evaluate(() => {
        const ws = (window as unknown as { __ws?: WebSocket }).__ws;
        if (ws) ws.close();
      });
      await page.waitForTimeout(200);
    }

    await page.waitForTimeout(1000);
    await page.goto(`/play.html?code=${code}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 15000 });
    await page.waitForFunction(
      () => (window as unknown as { state?: { nicknameSubmitted?: boolean } }).state?.nicknameSubmitted === true,
      { timeout: 10000 },
    );
  });

  test('zero-length WebSocket frame does not crash', async ({ page }) => {
    const code = await createRoom(page);
    await connectToRoom(page, code);
    await submitNickname(page, 'player1');

    await page.evaluate(() => {
      const ws = (window as unknown as { __ws?: WebSocket }).__ws;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send('');
      }
    });

    await page.waitForTimeout(500);
    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#nickname-setup-screen')).toHaveClass(/hidden/);
  });
});
