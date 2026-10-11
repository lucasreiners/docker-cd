import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './specs',
  testMatch: 'gitops.spec.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 180_000,
  expect: { timeout: 30_000 },
  outputDir: 'test-results/gitops',
  reporter: [['list'], ['html', { outputFolder: 'playwright-report/gitops', open: 'never' }]],
  use: {
    baseURL: 'http://127.0.0.1:18081',
    browserName: 'chromium',
    launchOptions: { slowMo: Number(process.env.E2E_SLOW_MO || 0) },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: [
    {
      command: 'go run -tags integration ./tests/integration/testserver',
      cwd: '../../backend',
      url: 'http://127.0.0.1:18080/test/ready',
      timeout: 180_000,
      reuseExistingServer: false,
      gracefulShutdown: { signal: 'SIGTERM', timeout: 30_000 },
      env: { GIN_MODE: 'release' },
    },
    {
      command: 'bun run dev --host 127.0.0.1 --port 18081 --strictPort',
      cwd: '../../frontend',
      url: 'http://127.0.0.1:18081',
      reuseExistingServer: false,
      env: { VITE_API_PROXY_TARGET: 'http://127.0.0.1:18080' },
    },
  ],
})
