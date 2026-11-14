// Dates and money in the venue's conventions: WIB, Indonesian names, "19.00", "Rp80.000".

export const venueTimeZone = "Asia/Jakarta";

const dayKeyFormat = new Intl.DateTimeFormat("en-CA", {
  timeZone: venueTimeZone,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});
const timeFormat = new Intl.DateTimeFormat("id-ID", {
  timeZone: venueTimeZone,
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
});
const longDateFormat = new Intl.DateTimeFormat("id-ID", {
  timeZone: venueTimeZone,
  weekday: "long",
  day: "numeric",
  month: "long",
  year: "numeric",
});
const thousands = new Intl.NumberFormat("id-ID");

/** The calendar day in the venue's zone, as YYYY-MM-DD. */
export function dayKey(moment: Date): string {
  return dayKeyFormat.format(moment);
}

/** The next `count` venue days starting today, as YYYY-MM-DD. */
export function upcomingDays(now: Date, count: number): string[] {
  const [year, month, day] = dayKey(now).split("-").map(Number);
  return Array.from({ length: count }, (_, offset) =>
    new Date(Date.UTC(year, month - 1, day + offset)).toISOString().slice(0, 10),
  );
}

/** "19.00" */
export function timeLabel(iso: string): string {
  return timeFormat.format(new Date(iso)).replace(":", ".");
}

/** "Sabtu, 14 November 2026" */
export function longDate(iso: string): string {
  return longDateFormat.format(new Date(iso));
}

/** Parts of a day key for the date strip, e.g. { weekday: "Sab", day: "14", month: "Nov" }. */
export function dayParts(key: string): { weekday: string; day: string; month: string } {
  // Noon UTC is the same calendar day in WIB, so the labels never slip a day.
  const noon = new Date(`${key}T12:00:00Z`);
  const part = (options: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat("id-ID", { timeZone: "UTC", ...options }).format(noon);
  return {
    weekday: part({ weekday: "short" }),
    day: part({ day: "numeric" }),
    month: part({ month: "short" }),
  };
}

/** "Rp80.000" */
export function rupiah(amount: number): string {
  return `Rp${thousands.format(amount)}`;
}
