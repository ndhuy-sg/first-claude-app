import fs from "node:fs";
import { defineConfig, devices, webkit } from "@playwright/test";

/**
 * Playwright e2e for Mộc POS.
 *
 *   api/scripts/test-db.sh start                 # local PostgreSQL (or set TEST_DATABASE_URL)
 *   cd web && npx playwright test                # starts the API (fresh DB) and Next.js itself
 *   npx playwright test --project=tablet-1180 e2e/flows/f01-sale.spec.ts
 *
 * Projects (test-case section W/F/N):
 *   tablet-1180   Chromium, iPad landscape 1180×820, touch   → POS specs + full flows
 *   tablet-1280   Chromium, Android landscape 1280×800, touch → POS specs
 *   desktop-1280  Chromium desktop 1280×800                  → admin specs + public invoice lookup
 *   webkit-ipad   WebKit (Safari engine) 1180×820, touch     → POS specs + full flows; only when the
 *                 WebKit browser is installed (CI: npx playwright install --with-deps webkit chromium)
 *   api           no browser: API-level checks (N-03)
 *
 * All projects share ONE seeded database, so tests run one at a time (workers: 1) and every spec
 * cleans up what it opens (shifts, orders, locks) or asserts on deltas, never on absolute totals.
 */

const PORT = Number(process.env.E2E_WEB_PORT ?? 3000);
const baseURL = process.env.E2E_BASE_URL ?? `http://localhost:${PORT}`;
const CI = !!process.env.CI;

const webkitInstalled = (() => {
  try {
    return fs.existsSync(webkit.executablePath());
  } catch {
    return false;
  }
})();

const posSpecs = ["pos/**/*.spec.ts"];
const flowSpecs = ["flows/**/*.spec.ts"];
const apiOnly = ["flows/security.spec.ts"];

const tablet = (width: number, height: number) => ({
  viewport: { width, height },
  hasTouch: true,
  isMobile: false,
  deviceScaleFactor: 2,
});

export default defineConfig({
  testDir: "./e2e",
  outputDir: "./test-results",
  fullyParallel: false,
  workers: 1,
  forbidOnly: CI,
  retries: CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: CI ? [["github"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL,
    locale: "vi-VN",
    timezoneId: "Asia/Ho_Chi_Minh",
    trace: (process.env.E2E_TRACE as "on" | "off" | "retain-on-failure" | undefined) ?? "off",
    screenshot: "only-on-failure",
    actionTimeout: 10_000,
  },
  projects: [
    {
      name: "tablet-1180",
      testMatch: [...posSpecs, ...flowSpecs],
      testIgnore: apiOnly,
      use: { ...devices["Desktop Chrome"], ...tablet(1180, 820) },
    },
    {
      name: "tablet-1280",
      testMatch: posSpecs,
      use: { ...devices["Desktop Chrome"], ...tablet(1280, 800) },
    },
    {
      name: "desktop-1280",
      testMatch: ["admin/**/*.spec.ts", "lookup.spec.ts"], // lookup.spec.ts sets its own phone viewport
      use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 800 } },
    },
    ...(webkitInstalled
      ? [
          {
            name: "webkit-ipad",
            testMatch: [...posSpecs, ...flowSpecs],
            testIgnore: apiOnly,
            use: { ...devices["Desktop Safari"], ...tablet(1180, 820) },
          },
        ]
      : []),
    {
      name: "api",
      testMatch: apiOnly,
    },
  ],
  webServer: [
    {
      // Go API on :8080 against a fresh migrated + seeded database (see the script).
      command: "bash e2e/support/start-stack.sh",
      url: "http://localhost:8080/api/v1/healthz",
      reuseExistingServer: !CI,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      // Next.js on :3000: `next dev` locally, the production standalone server in CI (E2E_WEB_MODE=prod).
      command: "bash e2e/support/start-web.sh",
      url: `${baseURL}/favicon.ico`,
      reuseExistingServer: !CI,
      timeout: 180_000,
      env: { E2E_WEB_PORT: String(PORT) },
    },
  ],
});
