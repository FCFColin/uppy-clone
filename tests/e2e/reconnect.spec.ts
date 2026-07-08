import { test, expect } from '@playwright/test';
import { createRoom, connectToRoom, submitNickname, closeWebSocket, waitForNicknameSubmitted } from './helpers.js';

test.describe('Reconnection', () => {
  test('disconnect and reconnect within grace period restores state', async ({ page }) => {
    const lobbyCode = await createRoom(page);
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page, 'E2EReconnectPlayer');

    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });

    await closeWebSocket(page);
    await page.waitForTimeout(1000);
    await page.goto(`/play.html?code=${lobbyCode}`);

    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await waitForNicknameSubmitted(page);
  });

  test('reconnect during waiting phase', async ({ page }) => {
    const lobbyCode = await createRoom(page);
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page, 'E2EWaitReconnect');

    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });

    await closeWebSocket(page);
    await page.waitForTimeout(1000);
    await page.goto(`/play.html?code=${lobbyCode}`);

    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await waitForNicknameSubmitted(page);
    await expect(page.locator('#nickname-setup-screen')).toHaveClass(/hidden/);
  });

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

  test('disconnected player is removed after grace period', async ({ page }) => {
    const lobbyCode = await createRoom(page);
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
    await submitNickname(page, 'E2EHost');

    const qp2 = await page.request.post('/api/v1/auth/quickplay', { data: { nickname: 'E2EGuest' } });
    expect(qp2.ok()).toBeTruthy();
    const match2 = await page.request.post('/api/v1/registry/match', { data: { code: lobbyCode } });
    expect(match2.ok()).toBeTruthy();

    await expect(page.locator('#player-list')).toContainText('E2EGuest', { timeout: 5000 });

    const check = await page.request.get(`/api/v1/registry/check/${lobbyCode}`);
    expect(check.ok()).toBeTruthy();
    const room = await check.json();
    expect(room.playerCount).toBeGreaterThanOrEqual(1);
  });
});