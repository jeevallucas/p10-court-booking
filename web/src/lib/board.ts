import type { OwnerBooking } from "@/api/client";

import { timeLabel } from "./format";

// The owner board's grid: half-hour rows from opening to closing, in WIB. The API enforces
// the same hours (outside_opening_hours); the venue has one set, so the web app keeps a copy.
export const opensAt = 7 * 60;
export const closesAt = 23 * 60;
export const slotMinutes = 30;
export const rowCount = (closesAt - opensAt) / slotMinutes;

// WIB is UTC+7 all year; Indonesia has no daylight saving.
const venueOffsetMinutes = 7 * 60;

/** Minutes since midnight in the venue, e.g. 19.30 WIB is 1170. */
export function venueMinute(iso: string): number {
  const utcMinute = Math.floor(Date.parse(iso) / 60_000);
  return (((utcMinute + venueOffsetMinutes) % 1440) + 1440) % 1440;
}

/** The board row a time falls in; 0 is 07.00. */
export function rowOf(iso: string): number {
  return (venueMinute(iso) - opensAt) / slotMinutes;
}

/** The start time of a row on a venue day, as an ISO string in WIB. */
export function rowStart(day: string, row: number): string {
  const minute = opensAt + row * slotMinutes;
  const hh = String(Math.floor(minute / 60)).padStart(2, "0");
  const mm = String(minute % 60).padStart(2, "0");
  return `${day}T${hh}:${mm}:00+07:00`;
}

/** "19.30" for a row. */
export function rowLabel(row: number): string {
  const minute = opensAt + row * slotMinutes;
  return `${String(Math.floor(minute / 60)).padStart(2, "0")}.${String(minute % 60).padStart(2, "0")}`;
}

export function rowsSpanned(booking: Pick<OwnerBooking, "start" | "end">): number {
  return (Date.parse(booking.end) - Date.parse(booking.start)) / (slotMinutes * 60_000);
}

export interface Placement {
  courtId: string;
  start: string;
  end: string;
}

/**
 * Where a booking lands when moved to `row` on `courtId`, keeping its length. Null when it
 * would run past closing time.
 */
export function moveTo(
  booking: OwnerBooking,
  day: string,
  courtId: string,
  row: number,
): Placement | null {
  const length = rowsSpanned(booking);
  if (row < 0 || row + length > rowCount) {
    return null;
  }
  return { courtId, start: rowStart(day, row), end: rowStart(day, row + length) };
}

// The statuses that keep a court busy, as in the database's exclusion constraint.
const occupying: ReadonlySet<OwnerBooking["status"]> = new Set(["held", "confirmed", "walk_in"]);

/** The first other booking that would overlap `placement` on its court, if any. */
export function conflictWith(
  bookings: OwnerBooking[],
  placement: Placement,
  ignoreId?: string,
): OwnerBooking | undefined {
  const start = Date.parse(placement.start);
  const end = Date.parse(placement.end);
  return bookings.find(
    (other) =>
      other.id !== ignoreId &&
      other.courtId === placement.courtId &&
      occupying.has(other.status) &&
      Date.parse(other.start) < end &&
      start < Date.parse(other.end),
  );
}

/**
 * Why a booking cannot go to `placement`, in words for the owner, or undefined when it can.
 * The API checks the same rules; this lets the board say so before sending anything.
 */
export function placementProblem(
  bookings: OwnerBooking[],
  placement: Placement | null,
  now: Date,
  ignoreId?: string,
): string | undefined {
  if (!placement) {
    return "Waktunya melewati jam tutup 23.00.";
  }
  if (Date.parse(placement.end) <= now.getTime()) {
    return "Jam itu sudah lewat.";
  }
  const other = conflictWith(bookings, placement, ignoreId);
  if (other) {
    return `Bentrok dengan ${other.customerName}, ${timeLabel(other.start)}–${timeLabel(other.end)}.`;
  }
  return undefined;
}

/** Only bookings that are settled and not over can be moved; the API refuses the rest. */
export function canMove(booking: OwnerBooking, now: Date): boolean {
  return (
    (booking.status === "confirmed" || booking.status === "walk_in") &&
    Date.parse(booking.end) > now.getTime()
  );
}

export const statusLabels: Record<OwnerBooking["status"], string> = {
  held: "Menunggu DP",
  confirmed: "Terkonfirmasi",
  walk_in: "Walk-in",
  no_show: "Tidak datang",
  cancelled: "Dibatalkan",
  expired: "Kedaluwarsa",
};
