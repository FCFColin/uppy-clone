import { test, expect } from '@playwright/test';

test.describe('Performance Smoke Tests', () => {
  test('page loads within acceptable time', async ({ page }) => {
    const startTime = Date.now();
    await page.goto('/');
    await page.waitForSelector('body', { timeout: 10000 });
    const loadTime = Date.now() - startTime;

    // 页面加载时间应在 10 秒以内
    expect(loadTime).toBeLessThan(10000);
  });

  test('no excessive console errors on load', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto('/');
    await page.waitForSelector('body', { timeout: 10000 });

    // 允许少量非致命错误，但不应该有大量错误
    expect(errors.length).toBeLessThan(5);
  });
});
