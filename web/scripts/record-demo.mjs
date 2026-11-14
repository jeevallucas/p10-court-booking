// Records the two README demos as animated GIFs: a player booking a court on a phone, and the
// owner moving bookings on a laptop, including a move the board refuses.
//
// Usage: node scripts/record-demo.mjs <output-dir>
// Needs the web app on WEB_URL (default http://localhost:3000) and the API behind it, freshly
// seeded. It books one real court hour.
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

import { chromium } from "@playwright/test";
import sharp from "sharp";

const base = process.env.WEB_URL ?? "http://localhost:3000";
const out = process.argv[2];
if (!out) {
  console.error("Give an output folder: node scripts/record-demo.mjs <output-dir>");
  process.exit(2);
}
await mkdir(out, { recursive: true });

const browser = await chromium.launch();
const tomorrow = new Date(Date.now() + 24 * 3600_000).toLocaleDateString("en-CA", {
  timeZone: "Asia/Jakarta",
});

/** Frames for one GIF: a screenshot and how long it stays on screen. */
function film() {
  const frames = [];
  return {
    frames,
    async take(page, seconds) {
      frames.push({ image: await page.screenshot(), delay: Math.round(seconds * 1000) });
    },
  };
}

async function saveGif(frames, file, width) {
  const images = await Promise.all(
    frames.map(({ image }) => sharp(image).resize({ width }).png().toBuffer()),
  );
  await sharp(images, { join: { animated: true } })
    .gif({ delay: frames.map(({ delay }) => delay), loop: 0, colours: 96, effort: 10 })
    .toFile(join(out, file));
}

async function recordPlayer() {
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    locale: "id-ID",
    timezoneId: "Asia/Jakarta",
  });
  const page = await context.newPage();
  const clip = film();
  const freeCourt = page.getByRole("button", { name: /per jam, pukul/ }).first();

  await page.goto(base);
  await freeCourt.waitFor();
  await clip.take(page, 2);
  await page.getByRole("region", { name: "Pilih tanggal" }).getByRole("button").nth(1).click();
  await freeCourt.waitFor();
  await clip.take(page, 1.5);
  await freeCourt.click();
  const sheet = page.getByRole("dialog");
  await sheet.waitFor();
  await clip.take(page, 1.5);
  await sheet.getByLabel("Nama pemesan").fill("Sari Wulandari");
  await sheet.getByLabel(/Nomor WhatsApp/).fill("0800 0000 4321");
  await clip.take(page, 2);
  await sheet.getByRole("button", { name: /^Bayar DP/ }).click();
  await page.getByRole("heading", { name: "Simulasi pembayaran DP" }).waitFor();
  await clip.take(page, 2);
  await page.getByRole("button", { name: /^Bayar Rp/ }).click();
  await page.getByText("Booking terkonfirmasi").waitFor();
  await clip.take(page, 3.5);

  await saveGif(clip.frames, "demo-player.gif", 390);
  await context.close();
}

async function recordOwner() {
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    locale: "id-ID",
    timezoneId: "Asia/Jakarta",
  });
  const page = await context.newPage();
  const clip = film();

  await page.goto(`${base}/owner/login`);
  await page.getByRole("button", { name: "Isi akun demo" }).click();
  await clip.take(page, 1.5);
  await page.getByRole("button", { name: "Masuk", exact: true }).click();
  await page.waitForURL("**/owner");
  await page.getByRole("button", { name: "Hari berikutnya" }).click();
  await page.locator("[data-status]").first().waitFor();
  await clip.take(page, 2);

  const response = await page.request.get(`${base}/v1/owner/bookings?date=${tomorrow}`);
  const day = await response.json();
  const { move, conflict } = pickMoves(day);

  // A move to a free court goes through at once.
  await drag(page, clip, move.booking, move.courtId);
  await page.getByText(/dipindah ke/).waitFor();
  await clip.take(page, 2.5);

  // A move onto another booking is refused before anything is sent.
  await drag(page, clip, conflict.booking, conflict.courtId);
  await page.getByText(/tidak dipindah/).waitFor();
  await clip.take(page, 3);

  await saveGif(clip.frames, "demo-owner.gif", 960);
  await context.close();
}

/** Drags a booking's block sideways onto another court's column, filming the landing spot. */
async function drag(page, clip, booking, courtId) {
  const block = page
    .locator(`[data-court-id="${booking.courtId}"] [data-status]`)
    .filter({ hasText: booking.customerName })
    .first();
  const from = await block.boundingBox();
  const column = await page.locator(`[data-court-id="${courtId}"]`).boundingBox();
  await page.mouse.move(from.x + from.width / 2, from.y + 8);
  await page.mouse.down();
  await page.mouse.move(column.x + column.width / 2, from.y + 8, { steps: 10 });
  await clip.take(page, 1.5);
  await page.mouse.up();
}

/**
 * From tomorrow's morning (on screen without scrolling): one confirmed booking with a free
 * court of its sport at the same time, and one with a busy court at that time.
 */
function pickMoves(day) {
  const occupying = new Set(["held", "confirmed", "walk_in"]);
  const overlaps = (a, b) =>
    Date.parse(a.start) < Date.parse(b.end) && Date.parse(b.start) < Date.parse(a.end);
  const hour = (iso) =>
    Number(
      new Date(iso).toLocaleString("en-GB", {
        timeZone: "Asia/Jakarta",
        hour: "2-digit",
        hourCycle: "h23",
      }),
    );
  let move;
  let conflict;
  for (const booking of day.bookings) {
    if (booking.status !== "confirmed" || hour(booking.start) > 12) {
      continue;
    }
    for (const court of day.courts) {
      if (court.sport !== booking.sport || court.id === booking.courtId) {
        continue;
      }
      const busy = day.bookings.some(
        (other) =>
          other.courtId === court.id && occupying.has(other.status) && overlaps(other, booking),
      );
      if (!busy && !move) {
        move = { booking, courtId: court.id };
      } else if (busy && !conflict && move?.booking.id !== booking.id) {
        conflict = { booking, courtId: court.id };
      }
    }
  }
  if (!move || !conflict) {
    throw new Error("Tomorrow's morning has no pair of moves to film; reseed the database.");
  }
  return { move, conflict };
}

await recordPlayer();
await recordOwner();
await browser.close();
console.log(`Saved demo-player.gif and demo-owner.gif to ${out}`);
