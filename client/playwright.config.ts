import { defineConfig } from '@playwright/test';

// CHROMIUM=/usr/bin/chromium locally; CI uses the bundled download
const chromium = process.env.CHROMIUM || undefined;

export default defineConfig({
  testDir: './e2e',
  timeout: 240_000,
  workers: 1,
  reporter: 'list',
  use: {
    viewport: { width: 1280, height: 720 },
    launchOptions: {
      executablePath: chromium,
      args: [
        '--no-sandbox',
        '--use-angle=swiftshader',
        '--enable-unsafe-swiftshader',
        '--disable-extensions',
        '--no-first-run',
        '--no-proxy-server',
      ],
    },
  },
  webServer: [
    {
      command: 'npm run dev -- --port 5173 --strictPort',
      cwd: '.',
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: true,
      timeout: 60_000,
    },
    {
      command: '../bin/rts-server -addr 127.0.0.1:8787 -fast',
      url: 'http://127.0.0.1:8787/healthz',
      reuseExistingServer: true,
      timeout: 30_000,
    },
  ],
});
