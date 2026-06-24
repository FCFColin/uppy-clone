import { defineConfig, devices } from '@playwright/test';

/**
 * 临时配置：针对 PRODUCTION URL 测试，不启动本地 wrangler dev。
 * 运行后请删除。
 */
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: 'https://uppy-clone.ekdjdn042.workers.dev',
    trace: 'off',
    screenshot: 'only-on-failure',
    timeout: 600000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // 不配置 webServer，避免启动本地 wrangler dev
});
