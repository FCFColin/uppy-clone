import { test, expect } from '@playwright/test';

const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || 'DevAdmin2024!Secure';

test.describe('Admin Flow', () => {
  test('admin login with credentials', async ({ page }) => {
    await page.goto('/admin.html');
    await page.waitForSelector('body', { timeout: 10000 });

    await expect(page.locator('#login-section:not(.hidden)')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#config-section')).toHaveClass(/hidden/);

    await page.fill('#admin-password', ADMIN_PASSWORD);
    await page.click('#login-btn');

    await expect(page.locator('#config-section:not(.hidden)')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#login-section')).toHaveClass(/hidden/);
  });

  test('config panel read after login', async ({ page }) => {
    await page.goto('/admin.html');
    await page.fill('#admin-password', ADMIN_PASSWORD);
    await page.click('#login-btn');
    await expect(page.locator('#config-section:not(.hidden)')).toBeVisible({ timeout: 5000 });

    const configRes = await page.request.get('/api/v1/admin/config');
    expect(configRes.ok()).toBeTruthy();
    const config = await configRes.json();
    expect(config).toHaveProperty('emailEnabled');
    expect(config).toHaveProperty('emailFrom');
  });

  test('admin logout clears session', async ({ page }) => {
    await page.goto('/admin.html');
    await page.fill('#admin-password', ADMIN_PASSWORD);
    await page.click('#login-btn');
    await expect(page.locator('#config-section:not(.hidden)')).toBeVisible({ timeout: 5000 });

    const logoutRes = await page.request.post('/api/v1/admin/logout');
    expect(logoutRes.ok()).toBeTruthy();

    await page.reload();
    await expect(page.locator('#login-section:not(.hidden)')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#config-section')).toHaveClass(/hidden/);
  });
});
