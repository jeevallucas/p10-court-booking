// Captures every player and owner screen and state at several widths, and runs axe on the
// main ones, for the design review loop (docs/design-review.md) and the README screenshots.
//
// Usage: node scripts/capture-screens.mjs <output-dir>
// Needs the web app on WEB_URL (default http://localhost:3000) and the API behind it, seeded.
// It places up to two real holds, which expire on their own after 10 minutes.
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
// The owner works on a laptop; 1366 x 768 is the size the design brief plans the board for.
const ownerViewports = [...viewports, { name: "1366", width: 1366, height: 768 }];

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

// A fresh number in the unassigned +62 800 range: one phone may hold only two slots at a time.
function testPhone() {
  return `0800${String(Math.floor(Math.random() * 1e8)).padStart(8, "0")}`;
}

function venueDay(offsetDays) {
  return new Date(Date.now() + offsetDays * 24 * 3600_000).toLocaleDateString("en-CA", {
    timeZone: "Asia/Jakarta",
  });
}

async function tomorrowSchedule(page) {
  await page.goto(base);
  await page.locator("[aria-pressed]").nth(1).click();
  await page
    .getByRole("button", { name: /per jam, pukul/ })
    .first()
    .waitFor();
}

await mkdir(out, { recursive: true });
const hold = await placeHold(venueDay(1), "Dewi Lestari", testPhone());
// A hold today puts a "Menunggu DP" block on the owner's board; late at night there may be none.
await placeHold(venueDay(0), "Bayu Pratama", testPhone()).catch((error) =>
  console.warn(`No hold today, so the board shows no held booking: ${error.message}`),
);

for (const viewport of viewports) {
  await capturePlayer(viewport);
}
for (const viewport of ownerViewports) {
  await captureOwner(viewport);
}

await browser.close();
await writeFile(join(out, "axe.json"), JSON.stringify(axeResults, null, 2));
const total = Object.values(axeResults).reduce((sum, list) => sum + list.length, 0);
console.log(`Saved screenshots to ${out}; axe violations: ${total}`);

async function capturePlayer(viewport) {
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

async function captureOwner(viewport) {
  const context = await browser.newContext({ viewport, locale: "id-ID" });
  const page = await context.newPage();
  const at = (label) => `${label}-${viewport.name}`;

  await page.goto(`${base}/owner/login`);
  await page.getByRole("heading", { level: 1 }).waitFor();
  await shot(page, at("11-owner-login"), { axe: true });
  await page.getByRole("button", { name: "Isi akun demo" }).click();
  await page.getByRole("button", { name: "Masuk", exact: true }).click();
  await page.waitForURL("**/owner");
  await page.locator("[data-status]").first().waitFor();
  await page.waitForLoadState("networkidle");
  await shot(page, at("12-owner-board"), { axe: true });

  for (let press = 0; press < 6; press += 1) {
    await page.keyboard.press("Tab");
  }
  await page.screenshot({ path: join(out, `${at("13-owner-keyboard-focus")}.png`) });

  const held = page.locator('[data-status="held"]').first();
  const chosen = (await held.count()) ? held : page.locator('[data-movable="true"]').first();
  await chosen.scrollIntoViewIfNeeded();
  await chosen.click();
  await page.getByRole("button", { name: "Tutup" }).waitFor();
  await shot(page, at("14-owner-panel"), { axe: true });
  await page.getByRole("button", { name: "Tutup" }).click();

  await page.getByRole("button", { name: "Tambah walk-in" }).click();
  const save = page.getByRole("button", { name: "Simpan walk-in" });
  // Saving without a name shows the form's errors; when every court is busy it stays disabled.
  if (await save.isEnabled()) {
    await save.click();
  }
  await shot(page, at("15-owner-walk-in"), { axe: true });
  await page.getByRole("button", { name: "Tutup" }).click();

  if (viewport.width >= 1024) {
    await dragIntoConflict(page, at);
  }

  await page.route("**/v1/owner/bookings**", async (route) => {
    const response = await route.fetch();
    const day = await response.json();
    await route.fulfill({ response, json: { ...day, bookings: [] } });
  });
  await page.reload();
  await page.getByText(/Belum ada booking/).waitFor();
  await shot(page, at("18-owner-empty"));
  await page.unrouteAll({ behavior: "ignoreErrors" });

  await page.route("**/v1/owner/bookings**", (route) => route.abort("connectionfailed"));
  await page.reload();
  await page.getByText(/Jadwal gagal dimuat/).waitFor({ timeout: 10_000 });
  await shot(page, at("19-owner-error"));
  await page.unrouteAll({ behavior: "ignoreErrors" });

  await page.route("**/v1/owner/bookings**", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 3000));
    await route.continue().catch(() => {});
  });
  await page.reload();
  await page.waitForTimeout(700);
  await shot(page, at("20-owner-loading"));
  await page.unrouteAll({ behavior: "ignoreErrors" });

  await context.close();
}

/** Drags a movable booking onto the next booking below it on the same court. */
async function dragIntoConflict(page, at) {
  const movable = page.locator('[data-movable="true"]');
  for (let index = 0; index < (await movable.count()); index += 1) {
    const block = movable.nth(index);
    await block.scrollIntoViewIfNeeded();
    const from = await block.boundingBox();
    const column = block.locator("xpath=..");
    const others = column.locator('[data-status]:not([data-status="no_show"])');
    for (let other = 0; other < (await others.count()); other += 1) {
      const to = await others.nth(other).boundingBox();
      const visible = to && to.y > from.y + from.height && to.y + 10 < page.viewportSize().height;
      if (!visible) {
        continue;
      }
      await page.mouse.move(from.x + from.width / 2, from.y + 6);
      await page.mouse.down();
      await page.mouse.move(to.x + to.width / 2, to.y + 12, { steps: 8 });
      await shot(page, at("16-owner-drag-conflict"));
      await page.mouse.up();
      await page.getByText(/tidak dipindah/).waitFor();
      await shot(page, at("17-owner-drop-refused"));
      return;
    }
  }
  console.warn("No visible pair of bookings to drag into a conflict; skipped the drag shots.");
}

/** Holds the first free badminton hour on `day` through the API, as a player would. */
async function placeHold(day, customerName, customerPhone) {
  const availability = await fetch(`${base}/v1/availability?date=${day}&sport=badminton`).then(
    (response) => response.json(),
  );
  const free = availability.slots.find(
    (slot) => slot.available && Date.parse(slot.start) > Date.now(),
  );
  if (!free) {
    throw new Error(`No free badminton hour on ${day}; reseed the database and retry.`);
  }
  const response = await fetch(`${base}/v1/holds`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      courtId: free.courtId,
      start: free.start,
      hours: 1,
      customerName,
      customerPhone,
    }),
  });
  if (!response.ok) {
    throw new Error(`Placing the review hold failed: ${response.status} ${await response.text()}`);
  }
  return (await response.json()).reference;
}
