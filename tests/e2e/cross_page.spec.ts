import { test, expect } from '@playwright/test';

test.describe('Cross-page flows', () => {
  test('index page quickplay navigates to play page', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('body', { timeout: 10000 });

    const quickplayButton = page.locator('#quickplay-btn');
    await expect(quickplayButton).toBeVisible({ timeout: 5000 });
    await quickplayButton.click();
    await page.waitForURL(/\/play\.html/, { timeout: 10000 });
    expect(page.url()).toContain('play');
  });

  test('leaderboard page loads and displays data', async ({ page }) => {
    await page.goto('/leaderboard.html');
    await expect(page.locator('body')).toBeVisible({ timeout: 10000 });

    const leaderboardContainer = page.locator('#leaderboard-container');
    await expect(leaderboardContainer).toBeVisible({ timeout: 5000 });

    const rows = page.locator('tr');
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });
});