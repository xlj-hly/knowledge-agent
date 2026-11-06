import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  workers: 1,
  retries: 0,
  timeout: 60_000,
  globalSetup: './e2e/setup.ts',
  use: { baseURL: 'http://localhost:5175', actionTimeout: 15_000, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  reporter: 'list',
  webServer: [
    { command: 'npm run dev -- --host 127.0.0.1 --port 5175 --strictPort', url: 'http://platform.localhost:5175', reuseExistingServer: false, env: { API_PROXY_TARGET: 'http://127.0.0.1:8081' } },
    { command: 'npm run dev:h5 -- --host 127.0.0.1 --port 5176 --strictPort', cwd: '../uniapp', url: 'http://localhost:5176', reuseExistingServer: false, env: { API_PROXY_TARGET: 'http://127.0.0.1:8081' } },
  ],
})
