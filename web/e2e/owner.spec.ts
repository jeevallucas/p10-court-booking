import { expect, test, type Page } from "@playwright/test";

import type { OwnerBooking } from "../src/api/client";
import { holdAsAnotherPlayer, ownerDay, ownerSessionFile, timeLabel, venueDay } from "./venue";

test.use({ storageState: ownerSessionFile });

const occupying = new Set(["held", "confirmed", "walk_in"]);
const closingMinute = 23 * 60;

async function openTomorrow(page: Page) {
  await page.goto("/owner");
  await page.getByRole("button", { name: "Hari berikutnya" }).click();
  await page.locator("[data-status]").first().waitFor();
}

/** The booking's block on the board, found by court, name and time. */
function block(page: Page, booking: OwnerBooking) {
  return page
    .locator(`[data-court-id="${booking.courtId}"] [data-status]`)
    .filter({ hasText: booking.customerName })
    .filter({ hasText: `${timeLabel(booking.start)}–${timeLabel(booking.end)}` });
}

function minutes(iso: string): number {
  const [hours, mins] = timeLabel(iso).split(".").map(Number);
  return hours * 60 + mins;
}

function length(booking: OwnerBooking): number {
  return (Date.parse(booking.end) - Date.parse(booking.start)) / 60_000;
}

/** A movable booking and the booking that follows it on the same court, close enough to see both. */
function bookingWithNeighbour(bookings: OwnerBooking[]) {
  for (const moved of bookings) {
    if (moved.status !== "confirmed" && moved.status !== "walk_in") {
      continue;
    }
    const inTheWay = bookings.find(
      (other) =>
        other.courtId === moved.courtId &&
        occupying.has(other.status) &&
        Date.parse(other.start) >= Date.parse(moved.end) &&
        Date.parse(other.start) - Date.parse(moved.end) <= 2 * 3600_000 &&
        minutes(other.start) + length(moved) <= closingMinute,
    );
    if (inTheWay) {
      return { moved, inTheWay };
    }
  }
  throw new Error("no movable booking with a neighbour tomorrow; reseed the database");
}

/** A movable booking and a free hour on another court of the same sport to move it to. */
function freeTargetFor(
  date: string,
  bookings: OwnerBooking[],
  courts: { id: string; sport: string }[],
) {
  for (const moved of bookings) {
    if (moved.status !== "confirmed" && moved.status !== "walk_in") {
      continue;
    }
    for (const court of courts) {
      if (court.sport !== moved.sport || court.id === moved.courtId) {
        continue;
      }
      for (let hour = 8; hour * 60 + length(moved) <= closingMinute; hour += 1) {
        const start = `${date}T${String(hour).padStart(2, "0")}:00:00+07:00`;
        const end = Date.parse(start) + length(moved) * 60_000;
        const taken = bookings.some(
          (other) =>
            other.courtId === court.id &&
            Date.parse(other.start) < end &&
            Date.parse(start) < Date.parse(other.end),
        );
        if (!taken) {
          return { moved, courtId: court.id, start, row: (hour * 60 - 7 * 60) / 30 };
        }
      }
    }
  }
  throw new Error("no free hour to move a booking to tomorrow; reseed the database");
}

test("dragging a booking onto another one is refused and nothing moves", async ({ page }) => {
  await openTomorrow(page);
  const day = await ownerDay(page.request, venueDay(1));
  const { moved, inTheWay } = bookingWithNeighbour(day.bookings);

  const dragged = block(page, moved);
  await dragged.scrollIntoViewIfNeeded();
  const from = await dragged.boundingBox();
  const to = await block(page, inTheWay).boundingBox();
  if (!from || !to) {
    throw new Error("the two bookings are not both on screen");
  }
  await page.mouse.move(from.x + from.width / 2, from.y + 6);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + 12, { steps: 8 });
  await page.mouse.up();

  await expect(page.getByText(/tidak dipindah\. Bentrok dengan/)).toBeVisible();
  await expect(dragged).toBeVisible();
});

test("a move the API refuses is put back, with the reason", async ({ page }) => {
  await openTomorrow(page);
  const date = venueDay(1);
  const day = await ownerDay(page.request, date);
  const target = freeTargetFor(date, day.bookings, day.courts);

  const moved = block(page, target.moved);
  await moved.scrollIntoViewIfNeeded();
  await moved.click();
  const panel = page.getByRole("complementary", { name: "Detail" });
  await panel.getByLabel("Lapangan").selectOption(target.courtId);
  await panel.getByLabel("Mulai").selectOption(String(target.row));

  // Another player takes that hour after the board loaded, so only the API knows of the clash.
  await holdAsAnotherPlayer(page.request, target.courtId, target.start);
  await panel.getByRole("button", { name: "Pindahkan booking" }).click();

  await expect(page.getByText(/baru saja terisi booking lain/)).toBeVisible();
  await expect(moved).toBeVisible();
});
