import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false, // 游戏测试需要顺序执行
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1, // 单线程避免端口冲突
  reporter: [['html'], ['list']],
  use: {
    baseURL: 'http://localhost:8787',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: process.env.PROD_E2E
    ? undefined
    : {
        command: 'npx wrangler dev',
        url: 'http://localhost:8787',
        reuseExistingServer: true,
        timeout: 30000,
      },
});
