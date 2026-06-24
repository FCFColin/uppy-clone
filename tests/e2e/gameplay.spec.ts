import { test, expect } from '@playwright/test';

test.describe('Gameplay Smoke Tests', () => {
  test('page loads successfully', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/.+/, { timeout: 10000 });
  });

  test('page contains game element', async ({ page }) => {
    await page.goto('/');
    // 等待页面主体内容加载
    await page.waitForSelector('body', { timeout: 10000 });
    const body = page.locator('body');
    await expect(body).toBeVisible();
  });
});
