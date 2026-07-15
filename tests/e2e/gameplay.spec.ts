import { test, expect } from '@playwright/test';
import { createRoom, connectToRoom, submitNickname, tapCanvas, waitForPhase, waitForNicknameSubmitted } from './helpers';

test.describe('Gameplay Main Flow', () => {
  test('quickplay auth and registry match', async ({ request }) => {
    const qp = await request.post('/api/v1/auth/quickplay', {
      data: { nickname: 'E2EPlayer' },
    });
    expect(qp.ok()).toBeTruthy();
    const body = await qp.json();
    expect(body).toHaveProperty('userId');
    expect(body).toHaveProperty('nickname');

    const match = await request.post('/api/v1/registry/match');
    expect(match.ok()).toBeTruthy();
    const { lobbyCode } = await match.json();
    expect(lobbyCode).toMatch(/^[A-Z2-9]{5}$/);

    const check = await request.get(`/api/v1/registry/check/${lobbyCode}`);
    expect(check.ok()).toBeTruthy();
    const room = await check.json();
    expect(room.code).toBe(lobbyCode);
    expect(room).toHaveProperty('playerCount');
  });

  test('enter game after ws connected shows waiting screen', async ({ page }) => {
    const code = await createRoom(page);
    await connectToRoom(page, code);
    await submitNickname(page, 'E2EConnectedPlayer');

    await expect(page.locator('#lobby-code')).toHaveText(code);
    await waitForNicknameSubmitted(page);
  });

  test('waiting screen stays after enter when WebSocket is slow', async ({ page }) => {
    const code = await createRoom(page);

    await page.route('**/api/v1/lobby/*/ws**', async (route) => {
      await new Promise((r) => setTimeout(r, 3000));
      await route.continue();
    });

    await page.goto(`/play.html?code=${code}`);
    await expect(page.locator('#nickname-setup-screen:not(.hidden)')).toBeVisible({ timeout: 15000 });

    await page.fill('#setup-nickname-input', 'E2ESlowPlayer');
    await page.click('#enter-game-btn');

    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#nickname-setup-screen')).toHaveClass(/hidden/);

    await page.waitForTimeout(3500);
    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible();
    await expect(page.locator('#nickname-setup-screen')).toHaveClass(/hidden/);
  });

  test('quickplay, lobby WebSocket connect, confirm nickname, and tap', async ({ page }) => {
    const code = await createRoom(page);

    await page.goto(`/play.html?code=${code}`);

    await page.waitForFunction(
      () => (window as unknown as { __ws?: WebSocket }).__ws?.readyState === WebSocket.OPEN,
      { timeout: 30000 },
    );

    const nicknameScreen = page.locator('#nickname-setup-screen:not(.hidden)');
    if (await nicknameScreen.isVisible().catch(() => false)) {
      await page.fill('#setup-nickname-input', 'E2ETapPlayer');
      await page.click('#enter-game-btn');
    }

    await waitForPhase(page, 'playing', 30000);
    await tapCanvas(page);

    await page.waitForFunction(
      () => {
        const s = (window as unknown as { state?: { score?: number; lastTapX?: number | null } }).state;
        return !!s && (s.score > 0 || s.lastTapX != null);
      },
      { timeout: 15000 },
    );
  });
});

test.describe('Gameplay Smoke Tests', () => {
  test('page loads successfully', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/.+/, { timeout: 10000 });
  });

  test('page contains quickplay button', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#quickplay-btn')).toBeVisible({ timeout: 10000 });
  });
});
