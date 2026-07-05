import { Page, expect } from '@playwright/test';

/** 快速认证并返回 userId 和 nickname */
export async function quickplayAuth(page: Page): Promise<{ userId: string; nickname: string }> {
  const res = await page.request.post('/api/v1/auth/quickplay', {
    data: { nickname: 'E2EPlayer' },
  });
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body).toHaveProperty('userId');
  expect(body).toHaveProperty('nickname');
  return { userId: body.userId, nickname: body.nickname };
}

/** 匹配房间并返回 lobbyCode */
export async function matchRoom(page: Page): Promise<string> {
  const match = await page.request.post('/api/v1/registry/match');
  expect(match.ok()).toBeTruthy();
  const { lobbyCode } = await match.json();
  expect(lobbyCode).toMatch(/^[A-Z2-9]{5}$/);
  return lobbyCode;
}

/** 导航到 play 页面并等待 WebSocket 连接 */
export async function connectToRoom(page: Page, code: string): Promise<void> {
  await page.goto(`/play.html?code=${code}`);
  await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });
}

/** 提交昵称并等待 waiting 页面 */
export async function submitNickname(page: Page, nickname: string): Promise<void> {
  await page.fill('#setup-nickname-input', nickname);
  await page.click('#enter-game-btn');
  await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });
  await expect(page.locator('#nickname-setup-screen')).toHaveClass(/hidden/);
}

/** 等待游戏阶段变更 */
export async function waitForPhase(page: Page, phase: string, timeout = 30000): Promise<void> {
  await page.waitForFunction(
    (p: string) => (window as unknown as { state?: { phase?: string } }).state?.phase === p,
    phase,
    { timeout },
  );
}

/** 模拟点击游戏画布中心 */
export async function tapCanvas(page: Page): Promise<void> {
  const canvas = page.locator('#game-canvas');
  await expect(canvas).toBeVisible();
  const box = await canvas.boundingBox();
  expect(box).not.toBeNull();
  if (box) {
    await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  }
}

/** 创建房间并返回 code（含快速认证和匹配） */
export async function createRoom(page: Page): Promise<string> {
  await quickplayAuth(page);
  return await matchRoom(page);
}

/** 快速认证+进入房间并提交昵称，返回 room code */
export async function createTestUser(page: Page, nickname: string): Promise<string> {
  const code = await createRoom(page);
  await connectToRoom(page, code);
  await submitNickname(page, nickname);
  return code;
}

/** 通过 UI 进入房间（导航 + WS 连接 + 昵称提交 + 等待 waiting 页面）返回 room code */
export async function createRoomViaUI(page: Page, nickname = 'host'): Promise<string> {
  return await createTestUser(page, nickname);
}