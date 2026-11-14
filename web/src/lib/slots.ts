import type { Availability, Court, Slot } from "@/api/client";

export interface FreeCourt {
  court: Court;
  slot: Slot;
}

export interface HourRow {
  start: string;
  free: FreeCourt[];
  /** True when the hour has started; past hours are hidden rather than shown as full. */
  past: boolean;
}

/** Groups the day's slots into hours, each listing the courts still free, in court order. */
export function hourRows(availability: Availability, now: Date): HourRow[] {
  const courtOrder = new Map(availability.courts.map((court, index) => [court.id, index]));
  const courts = new Map(availability.courts.map((court) => [court.id, court]));
  const byStart = new Map<string, HourRow>();
  for (const slot of availability.slots) {
    let row = byStart.get(slot.start);
    if (!row) {
      row = { start: slot.start, free: [], past: new Date(slot.start) <= now };
      byStart.set(slot.start, row);
    }
    const court = courts.get(slot.courtId);
    if (slot.available && court) {
      row.free.push({ court, slot });
    }
  }
  const rows = [...byStart.values()].sort((a, b) => a.start.localeCompare(b.start));
  for (const row of rows) {
    row.free.sort((a, b) => (courtOrder.get(a.court.id) ?? 0) - (courtOrder.get(b.court.id) ?? 0));
  }
  return rows;
}

/** Whether the same court is also free for the following hour, so two hours can be booked. */
export function nextHourFree(availability: Availability, slot: Slot): boolean {
  return availability.slots.some(
    (other) =>
      other.courtId === slot.courtId &&
      new Date(other.start).getTime() === new Date(slot.end).getTime() &&
      other.available,
  );
}

/** Half the price rounded up to a thousand Rupiah, as the API charges it. */
export function deposit(price: number): number {
  return Math.ceil(Math.ceil(price / 2) / 1000) * 1000;
}
