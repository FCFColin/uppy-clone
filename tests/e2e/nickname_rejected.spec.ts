import { test, expect } from '@playwright/test';
import { createRoom, connectToRoom, submitNickname, quickplayAuth, matchRoom } from './helpers.js';

test.describe('Nickname rejected and room reentry (spec: fix-room-entry-stuck-and-reentry)', () => {
  test('player submitting a duplicate nickname receives NICKNAME_REJECTED and can retry', async ({ page, context }) => {
    // 玩家 A：创建房间并提交昵称 "E2EDupName"
    const lobbyCode = await createRoom(page);
    await connectToRoom(page, lobbyCode);
    await submitNickname(page, 'E2EDupName');
    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });

    // 玩家 B：在另一个 page 中加入同一房间并尝试使用相同昵称
    const pageB = await context.newPage();
    await quickplayAuth(pageB);
    await connectToRoom(pageB, lobbyCode);

    // 提交重复昵称 - 应该被服务器拒绝
    await pageB.fill('#setup-nickname-input', 'E2EDupName');
    await pageB.click('#enter-game-btn');

    // 期望 B 回退到 nickname 步骤，显示「昵称已被占用」
    await expect(pageB.locator('#nickname-connect-status')).toContainText('昵称已被占用', { timeout: 5000 });
    // nickname-setup-screen 应该重新可见（不再 hidden）
    await expect(pageB.locator('#nickname-setup-screen')).not.toHaveClass(/hidden/, { timeout: 5000 });
    // waiting-screen 不应可见
    await expect(pageB.locator('#waiting-screen')).toHaveClass(/hidden/);

    // B 重新输入不同的昵称
    await pageB.fill('#setup-nickname-input', 'E2EDupName2');
    await pageB.click('#enter-game-btn');
    // 应该正常进入 waiting
    await expect(pageB.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });
    await expect(pageB.locator('#nickname-setup-screen')).toHaveClass(/hidden/);

    await pageB.close();
  });

  test('player stuck in waiting step sees timeout fallback panel after 15s', async ({ page }) => {
    // 这个测试通过 page.evaluate 直接将客户端状态置为 waiting 步骤，
    // 模拟服务器不响应 NICKNAME_REJECTED 也不发送 phase 转换的场景。
    // 验证 15 秒超时兜底会显示带操作按钮的全屏错误面板。
    const lobbyCode = await createRoom(page);
    await page.goto(`/play.html?code=${lobbyCode}`);
    await expect(page.locator('#nickname-connect-status')).toContainText('服务器已连接', { timeout: 30000 });

    // 通过 UI 进入 waiting 步骤（提交合法昵称）
    await page.fill('#setup-nickname-input', 'E2EWaitingTimeout');
    await page.click('#enter-game-btn');
    await expect(page.locator('#waiting-screen:not(.hidden)')).toBeVisible({ timeout: 5000 });

    // 拦截 WebSocket 消息：模拟服务器不再发送 GAME_STATE_CHANGE 也不发送 NICKNAME_REJECTED
    // 通过 stub WebSocket 的 onmessage 让其忽略 GAME_STATE_CHANGE 和 SNAPSHOT
    await page.evaluate(() => {
      const ws = (window as unknown as { __ws?: WebSocket }).__ws;
      if (ws) {
        // 保存原始 onmessage 然后替换为只处理 PONG 的版本
        const original = ws.onmessage;
        ws.onmessage = (ev: MessageEvent) => {
          try {
            const buf: ArrayBuffer = ev.data instanceof ArrayBuffer
              ? ev.data
              : (ev.data instanceof Blob ? null : null);
            if (buf && buf.byteLength >= 1) {
              const view = new DataView(buf);
              const msgType = view.getUint8(0);
              // 0x21 = PONG, 0x08 = NICKNAME_REJECTED（不期望收到，但若收到也忽略以模拟卡死）
              if (msgType === 0x21) {
                if (original) original.call(ws, ev);
                return;
              }
              // 忽略其他所有消息（SNAPSHOT / GAME_STATE_CHANGE 等）
              return;
            }
          } catch {
            // 解析失败则调用原始 handler
          }
          if (original) original.call(ws, ev);
        };
      }
    });

    // 等待 15 秒超时兜底触发（额外留 2 秒缓冲）
    // 期望出现全屏错误面板，包含「重新匹配」按钮
    await expect(page.locator('#loading-error-text, #loading-error-title')).toBeVisible({ timeout: 20000 });
    await expect(page.locator('#loading-match-btn')).toBeVisible({ timeout: 2000 });
  });
});
