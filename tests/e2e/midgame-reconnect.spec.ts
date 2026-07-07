import { test, expect } from '@playwright/test';
import { createRoom, connectToRoom, submitNickname, closeWebSocket, waitForNicknameSubmitted } from './helpers.js';

test.describe('Mid-game reconnect', () => {
  test('disconnect during playing phase and reconnect', async ({ page }) => {
    const lobbyCode = await createRoom(page);
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page, 'E2EPlayReconnect');

    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });

    await closeWebSocket(page);
    await page.waitForTimeout(1000);
    await page.goto(`/play.html?code=${lobbyCode}`);

    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await waitForNicknameSubmitted(page);
  });
});
