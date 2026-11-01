import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  use: {
    browserName: process.env.MIAOYIN_E2E_BROWSER === 'webkit' ? 'webkit' : 'chromium',
    baseURL: 'http://127.0.0.1:14873',
    viewport: { width: 1180, height: 800 },
    reducedMotion: 'reduce'
  },
  webServer: {
    command: 'npm run preview -- --host 127.0.0.1 --port 14873 --strictPort',
    url: 'http://127.0.0.1:14873',
    reuseExistingServer: false
  }
})
