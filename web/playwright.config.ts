import { defineConfig, devices } from '@playwright/test'

// Visual tests run against the Vite dev server; e2e/fixtures.ts mocks the API.
const port = Number(process.env.E2E_PORT ?? 5179)
const baseURL = `http://127.0.0.1:${port}`

export default defineConfig({
  testDir: './e2e',
  // The e2e Devflow check points this at its report folder.
  outputDir: process.env.E2E_OUTPUT_DIR ?? 'test-results',
  snapshotPathTemplate: '{testDir}/__screenshots__/{testFilePath}/{arg}-{projectName}-{platform}{ext}',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: process.env.E2E_JUNIT_FILE
    ? [['list'], ['junit', { outputFile: process.env.E2E_JUNIT_FILE }]]
    : [['list']],
  expect: {
    toHaveScreenshot: { animations: 'disabled', caret: 'hide' },
  },
  use: {
    baseURL,
    colorScheme: 'dark',
    locale: 'en-US',
    timezoneId: 'UTC',
    screenshot: 'on',
  },
  projects: [
    { name: 'phone', use: { ...devices['Pixel 5'], viewport: { width: 390, height: 844 } } },
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 } } },
  ],
  webServer: {
    command: `pnpm exec vite --host 127.0.0.1 --port ${port} --strictPort`,
    url: baseURL,
    reuseExistingServer: false,
  },
})
