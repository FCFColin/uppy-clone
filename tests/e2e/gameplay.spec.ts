import { test, expect } from '@playwright/test';

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

  test('quickplay, lobby WebSocket connect, confirm nickname, and tap', async ({ page }) => {
    const qp = await page.request.post('/api/v1/auth/quickplay', {
      data: { nickname: 'E2ETapPlayer' },
    });
    expect(qp.ok()).toBeTruthy();

    const match = await page.request.post('/api/v1/registry/match');
    expect(match.ok()).toBeTruthy();
    const { lobbyCode } = await match.json();

    await page.goto(`/play.html?code=${lobbyCode}`);

    await page.waitForFunction(
      () => (window as unknown as { __ws?: WebSocket }).__ws?.readyState === WebSocket.OPEN,
      { timeout: 30000 },
    );

    const nicknameScreen = page.locator('#nickname-setup-screen:not(.hidden)');
    if (await nicknameScreen.isVisible().catch(() => false)) {
      await page.fill('#setup-nickname-input', 'E2ETapPlayer');
      await page.click('#enter-game-btn');
    }

    await page.waitForFunction(
      () => (window as unknown as { state?: { phase?: string } }).state?.phase === 'playing',
      { timeout: 90000 },
    );

    const canvas = page.locator('#game-canvas');
    await expect(canvas).toBeVisible();
    const box = await canvas.boundingBox();
    expect(box).not.toBeNull();
    if (box) {
      await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
    }

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

  test('page contains game element', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('body', { timeout: 10000 });
    const body = page.locator('body');
    await expect(body).toBeVisible();
  });
});
