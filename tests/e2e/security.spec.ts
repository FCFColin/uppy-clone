import { test, expect } from '@playwright/test';

test.describe('security', () => {
  test('tampered auth cookie is rejected', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => {
      document.cookie = 'session=injected-tampered-value';
    });
    const check = await page.request.get('/api/v1/auth/check');
    expect(check.ok()).toBeFalsy();
  });

  test('XSS in nickname is sanitized', async ({ page }) => {
    const qp = await page.request.post('/api/v1/auth/quickplay', {
      data: { nickname: '<script>alert("xss")</script>' },
    });
    expect(qp.ok()).toBeTruthy();
    const body = await qp.json();
    expect(body.nickname).not.toContain('<script>');
  });

  test('rate limiting on rapid room code guessing', async ({ page }) => {
    const results: number[] = [];
    for (let i = 0; i < 50; i++) {
      const check = await page.request.get('/api/v1/registry/check/XXXXX');
      results.push(check.status());
    }
    const rateLimited = results.some((s) => s === 429);
    expect(rateLimited).toBeTruthy();
  });
});
