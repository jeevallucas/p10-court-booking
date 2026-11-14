import { defineConfig, devices } from "@playwright/test";

// End-to-end tests drive the built web app against the real API and a seeded PostgreSQL.
// Locally: start the database, seed it, build the web app, then `npm run e2e` (README); servers
// already running on ports 8080 and 3000 are reused. CI starts both from scratch.
const today = new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Jakarta" });

export default defineConfig({
  testDir: "./e2e",
  // The tests share one seeded database and book real slots, so they run one at a time.
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: "http://localhost:3000",
    locale: "id-ID",
    timezoneId: "Asia/Jakarta",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /\.setup\.ts$/ },
    {
      name: "chromium",
      dependencies: ["setup"],
      use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } },
    },
  ],
  webServer: [
    {
      command: "go run ./cmd/api",
      cwd: "../api",
      url: `http://localhost:8080/v1/availability?date=${today}&sport=badminton`,
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
      env: {
        DATABASE_URL:
          process.env.DATABASE_URL ??
          "postgres://court:court@localhost:5432/court_booking?sslmode=disable",
        PAYMENT_SERVER_KEY: "e2e-payment-key",
        COOKIE_SECURE: "false",
        PORT: "8080",
        // Lets the tests' "other player" bring its own address, as players behind a proxy do.
        TRUSTED_PROXY_HOPS: "1",
        PUBLIC_WEB_URL: "http://localhost:3000",
      },
    },
    {
      command: "npm run start",
      url: "http://localhost:3000/owner/login",
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
    },
  ],
});
