// Captures every player screen and state at three widths, and runs axe on each, for the
// design review loop (docs/design-review.md) and the README screenshots.
//
// Usage: node scripts/capture-screens.mjs <output-dir>
// Needs the web app on WEB_URL (default http://localhost:3000) and the API behind it, seeded.
// It places one real hold, which expires on its own after 10 minutes.
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

import AxeBuilder from "@axe-core/playwright";
import { chromium } from "@playwright/test";

const base = process.env.WEB_URL ?? "http://localhost:3000";
const out = process.argv[2];
if (!out) {
  console.error("Give an output folder: node scripts/capture-screens.mjs <output-dir>");
  process.exit(2);
}

const viewports = [
  { name: "375", width: 375, height: 812 },
  { name: "768", width: 768, height: 1024 },
  { name: "1440", width: 1440, height: 900 },
];

const browser = await chromium.launch();
const axeResults = {};

async function check(page, label) {
  const { violations } = await new AxeBuilder({ page }).analyze();
  axeResults[label] = violations.map((v) => ({
    id: v.id,
    impact: v.impact,
    nodes: v.nodes.map((n) => n.target.join(" ")),
  }));
}

async function shot(page, label, { axe = false } = {}) {
  await page.screenshot({ path: join(out, `${label}.png`), fullPage: true });
  if (axe) {
    await check(page, label);
  }
}

async function tomorrowSchedule(page) {
  await page.goto(base);
  await page.locator("[aria-pressed]").nth(1).click();
  await page
    .getByRole("button", { name: /per jam, pukul/ })
    .first()
    .waitFor();
}

const hold = await placeHold();

for (const viewport of viewports) {
  const context = await browser.newContext({ viewport, locale: "id-ID" });
  const page = await context.newPage();
  const at = (label) => `${label}-${viewport.name}`;

  await page.goto(base);
  await page.getByRole("heading", { level: 2 }).waitFor();
  await page.waitForLoadState("networkidle");
  await shot(page, at("01-today"), { axe: true });

  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await page.screenshot({ path: join(out, `${at("02-keyboard-focus")}.png`) });

  await tomorrowSchedule(page);
  await shot(page, at("03-tomorrow"));

  await page
    .getByRole("button", { name: /per jam, pukul/ })
    .first()
    .click();
  await page.getByRole("dialog").waitFor();
  await shot(page, at("04-booking-sheet"), { axe: true });
  await page.getByRole("button", { name: /Bayar DP/ }).click();
  await shot(page, at("05-booking-sheet-errors"));
  await page.keyboard.press("Escape");

  await page.route("**/v1/availability**", async (route) => {
    const response = await route.fetch();
    const availability = await response.json();
    availability.slots = availability.slots.map((slot) => ({ ...slot, available: false }));
    await route.fulfill({ response, json: availability });
  });
  await page.goto(base);
  await page.locator("[aria-pressed]").nth(1).click();
  await page.getByText(/penuh pada tanggal ini/).waitFor();
  await shot(page, at("06-all-full"));
  await page.unrouteAll({ behavior: "ignoreErrors" });

  await page.route("**/v1/availability**", (route) => route.abort("connectionfailed"));
  await page.goto(base);
  await page.getByText(/Jadwal gagal dimuat/).waitFor();
  await shot(page, at("07-error"));
  await page.unrouteAll({ behavior: "ignoreErrors" });

  await page.route("**/v1/availability**", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 3000));
    await route.continue().catch(() => {});
  });
  await page.goto(base);
  await page.waitForTimeout(700);
  await shot(page, at("08-loading"));
  await page.unrouteAll({ behavior: "ignoreErrors" });

  for (const status of ["held", "confirmed", "expired"]) {
    await page.route(`**/v1/bookings/${hold}`, async (route) => {
      const response = await route.fetch();
      const booking = await response.json();
      await route.fulfill({ response, json: { ...booking, status } });
    });
    await page.goto(`${base}/booking/${hold}`);
    await page.getByRole("heading", { level: 1 }).waitFor();
    await shot(page, at(`09-status-${status}`), { axe: status === "held" });
    await page.unrouteAll({ behavior: "ignoreErrors" });
  }

  await page.goto(`${base}/booking/ZZZZZZZZ`);
  await page.getByRole("heading", { name: /tidak ditemukan/ }).waitFor();
  await shot(page, at("10-status-not-found"));

  await context.close();
}

await browser.close();
await writeFile(join(out, "axe.json"), JSON.stringify(axeResults, null, 2));
const total = Object.values(axeResults).reduce((sum, list) => sum + list.length, 0);
console.log(`Saved screenshots to ${out}; axe violations: ${total}`);

/** Holds the first free hour tomorrow through the API, as a player would. */
async function placeHold() {
  await mkdir(out, { recursive: true });
  const tomorrow = new Date(Date.now() + 24 * 3600_000).toLocaleDateString("en-CA", {
    timeZone: "Asia/Jakarta",
  });
  const availability = await fetch(`${base}/v1/availability?date=${tomorrow}&sport=badminton`).then(
    (response) => response.json(),
  );
  const free = availability.slots.find((slot) => slot.available);
  if (!free) {
    throw new Error(`No free badminton hour on ${tomorrow}; reseed the database and retry.`);
  }
  const response = await fetch(`${base}/v1/holds`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      courtId: free.courtId,
      start: free.start,
      hours: 1,
      customerName: "Dewi Lestari",
      customerPhone: "081298765432",
    }),
  });
  if (!response.ok) {
    throw new Error(`Placing the review hold failed: ${response.status} ${await response.text()}`);
  }
  return (await response.json()).reference;
}
