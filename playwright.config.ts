import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.E2E_BASE_URL || 'http://localhost:57266';

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 1,
  workers: 4,
  reporter: [['html'], ['list']],
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],
  webServer: process.env.PROD_E2E
    ? undefined
    : {
        command: 'npm run build:frontend && cd backend && go run ./cmd/server',
        url: baseURL,
        reuseExistingServer: !process.env.CI,
        timeout: 120000,
        env: {
          ...process.env,
          ENABLE_HSTS: 'false',
          FRONTEND_DIR: 'frontend/dist',
          MIGRATIONS_DIR: 'backend/migrations',
        },
      },
});
