import { expect, test } from "@playwright/test";

import { availability, firstListedFreeSlot, holdAsAnotherPlayer, venueDay } from "./venue";

async function openTomorrow(page: import("@playwright/test").Page) {
  await page.goto("/");
  await page.getByRole("region", { name: "Pilih tanggal" }).getByRole("button").nth(1).click();
  await page
    .getByRole("button", { name: /per jam, pukul/ })
    .first()
    .waitFor();
}

test("a player holds a court, pays the deposit and sees the booking confirmed", async ({
  page,
}) => {
  await openTomorrow(page);
  await page
    .getByRole("button", { name: /per jam, pukul/ })
    .first()
    .click();

  const sheet = page.getByRole("dialog");
  await sheet.getByLabel("Nama pemesan").fill("Sari Wulandari");
  await sheet.getByLabel(/Nomor WhatsApp/).fill("0800 0000 4321");
  await sheet.getByRole("button", { name: /^Bayar DP/ }).click();

  await expect(page.getByRole("heading", { name: "Simulasi pembayaran DP" })).toBeVisible();
  await page.getByRole("button", { name: /^Bayar Rp/ }).click();

  await expect(page).toHaveURL(/\/booking\/[0-9A-Z]{8}$/);
  await expect(page.getByText("Booking terkonfirmasi")).toBeVisible();
  await expect(page.getByText(/dibayar di kasir/)).toBeVisible();
});

test("a player who picks an hour someone else just held is told so", async ({ page }) => {
  await openTomorrow(page);
  const day = await availability(page.request, venueDay(1));
  const slot = firstListedFreeSlot(day);

  // The schedule on screen still shows the hour as free when another player takes it.
  await holdAsAnotherPlayer(page.request, slot.courtId, slot.start);
  await page
    .getByRole("button", { name: /per jam, pukul/ })
    .first()
    .click();
  const sheet = page.getByRole("dialog");
  await sheet.getByLabel("Nama pemesan").fill("Agus Setiawan");
  await sheet.getByLabel(/Nomor WhatsApp/).fill("080000004322");
  await sheet.getByRole("button", { name: /^Bayar DP/ }).click();

  await expect(sheet.getByRole("alert")).toContainText("baru saja dipesan orang lain");
});
