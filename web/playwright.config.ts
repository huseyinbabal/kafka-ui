import { defineConfig, devices } from '@playwright/test'

/**
 * E2E tests run against a deployed instance (by default the kind deployment
 * behind `make kind-forward`). Override with KAFKAUI_E2E_URL.
 */
export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.ts',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.KAFKAUI_E2E_URL ?? 'http://127.0.0.1:18080',
    trace: 'retain-on-failure',
    storageState: 'e2e/.auth/admin.json',
    ...devices['Desktop Chrome'],
    viewport: { width: 1440, height: 900 },
  },
})
