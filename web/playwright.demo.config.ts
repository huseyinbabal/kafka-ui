import { defineConfig } from '@playwright/test'

/** Records the README demo video (see `make demo`). Not part of the test suite. */
export default defineConfig({
  testDir: './demo',
  timeout: 180_000,
  workers: 1,
  reporter: [['list']],
  outputDir: 'demo/out',
  use: {
    baseURL: process.env.KAFKAUI_E2E_URL ?? 'http://127.0.0.1:18080',
    viewport: { width: 1280, height: 800 },
    video: { mode: 'on', size: { width: 1280, height: 800 } },
    colorScheme: 'dark',
  },
})
