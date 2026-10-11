import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './specs',
  testMatch: 'redesign.spec.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  outputDir: 'test-results/redesign',
  reporter: [['list'], ['html', { outputFolder: 'playwright-report/redesign', open: 'never' }]],
  use: {
    baseURL: 'http://127.0.0.1:18083',
    browserName: 'chromium',
    viewport: { width: 1440, height: 1000 },
    colorScheme: 'light',
    launchOptions: { slowMo: Number(process.env.E2E_SLOW_MO || 0) },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: [
    {
      command: 'bun fixtures/redesign-server.ts',
      url: 'http://127.0.0.1:18082/test/ready',
      reuseExistingServer: false,
    },
    {
      command: 'bun run dev --host 127.0.0.1 --port 18083 --strictPort',
      cwd: '../../frontend',
      url: 'http://127.0.0.1:18083',
      reuseExistingServer: false,
      env: { VITE_API_PROXY_TARGET: 'http://127.0.0.1:18082' },
    },
  ],
})
