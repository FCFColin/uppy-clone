import { test, expect } from '@playwright/test';
import { quickplayAuth } from './helpers';

test.describe('Auth Flow', () => {
  test('quickplay auth sets valid session', async ({ page }) => {
    const { nickname } = await quickplayAuth(page);

    const check = await page.request.get('/api/v1/auth/check');
    expect(check.ok()).toBeTruthy();
    const body = await check.json();
    expect(body.authenticated).toBe(true);
    expect(body.nickname).toBe(nickname);
  });

  test('session persists after page refresh', async ({ page }) => {
    await quickplayAuth(page);

    const check1 = await page.request.get('/api/v1/auth/check');
    expect(check1.ok()).toBeTruthy();

    await page.reload();

    const check2 = await page.request.get('/api/v1/auth/check');
    expect(check2.ok()).toBeTruthy();
  });

  test('logout clears session', async ({ page }) => {
    await quickplayAuth(page);

    const check1 = await page.request.get('/api/v1/auth/check');
    expect(check1.ok()).toBeTruthy();

    const logoutRes = await page.request.post('/api/v1/auth/logout');
    expect(logoutRes.ok()).toBeTruthy();

    const check2 = await page.request.get('/api/v1/auth/check');
    expect(check2.ok()).toBeFalsy();
  });
});
