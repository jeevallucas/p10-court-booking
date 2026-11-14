import type { APIRequestContext } from "@playwright/test";

import type { Availability, OwnerDay } from "../src/api/client";

// Helpers shared by the end-to-end tests: dates and times in WIB, and direct API calls that
// stand in for another player acting at the same moment.

/** Where owner.setup.ts saves the demo owner's session for the owner tests. */
export const ownerSessionFile = "e2e/.auth/owner.json";

/**
 * A fresh number in the unassigned +62 800 range for each booking: the API lets one phone
 * hold two slots at a time, and repeated runs must not use that up.
 */
export function testPhone(): string {
  return `0800${String(Math.floor(Math.random() * 1e8)).padStart(8, "0")}`;
}

export function venueDay(offsetDays: number): string {
  return new Date(Date.now() + offsetDays * 24 * 3600_000).toLocaleDateString("en-CA", {
    timeZone: "Asia/Jakarta",
  });
}

const timeFormat = new Intl.DateTimeFormat("id-ID", {
  timeZone: "Asia/Jakarta",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});

/** "19.00", as the web app writes times. */
export function timeLabel(iso: string): string {
  return timeFormat.format(new Date(iso)).replace(":", ".");
}

export async function availability(
  request: APIRequestContext,
  date: string,
): Promise<Availability> {
  const response = await request.get(`/v1/availability?date=${date}&sport=badminton`);
  if (!response.ok()) {
    throw new Error(`availability for ${date}: ${response.status()} ${await response.text()}`);
  }
  return response.json();
}

/** The free hour the schedule lists first: the earliest one, then in court order. */
export function firstListedFreeSlot(day: Availability) {
  const courtOrder = day.courts.map((court) => court.id);
  const free = day.slots
    .filter((slot) => slot.available && Date.parse(slot.start) > Date.now())
    .sort(
      (a, b) =>
        Date.parse(a.start) - Date.parse(b.start) ||
        courtOrder.indexOf(a.courtId) - courtOrder.indexOf(b.courtId),
    );
  if (!free[0]) {
    throw new Error(`no free badminton hour on ${day.date}; reseed the database`);
  }
  return free[0];
}

/**
 * Holds a court for an hour as another player would: straight to the API, from an address of
 * their own (the API trusts one proxy hop, as in playwright.config.ts), so the test player's
 * per-address hold limit is not spent on someone else's booking.
 */
export async function holdAsAnotherPlayer(
  request: APIRequestContext,
  courtId: string,
  start: string,
): Promise<void> {
  const otherAddress = `10.30.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250)}`;
  const response = await request.post("http://localhost:8080/v1/holds", {
    headers: { "X-Forwarded-For": otherAddress },
    data: {
      courtId,
      start,
      hours: 1,
      customerName: "Pemain lain",
      customerPhone: testPhone(),
    },
  });
  if (response.status() !== 201) {
    throw new Error(
      `the other player's hold failed: ${response.status()} ${await response.text()}`,
    );
  }
}

export async function ownerDay(request: APIRequestContext, date: string): Promise<OwnerDay> {
  const response = await request.get(`/v1/owner/bookings?date=${date}`);
  if (!response.ok()) {
    throw new Error(`owner day ${date}: ${response.status()} ${await response.text()}`);
  }
  return response.json();
}
