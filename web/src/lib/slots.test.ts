import { describe, expect, it } from "vitest";

import type { Availability } from "@/api/client";
import { deposit, hourRows, nextHourFree } from "./slots";

const courtOne = { id: "c1", name: "Lapangan 1", sport: "badminton" as const };
const courtTwo = { id: "c2", name: "Lapangan 2", sport: "badminton" as const };

function slot(courtId: string, hour: number, available: boolean) {
  const pad = (n: number) => String(n).padStart(2, "0");
  return {
    courtId,
    start: `2026-11-20T${pad(hour)}:00:00+07:00`,
    end: `2026-11-20T${pad(hour + 1)}:00:00+07:00`,
    priceIdr: hour >= 17 ? 80000 : 60000,
    available,
  };
}

const availability: Availability = {
  date: "2026-11-20",
  sport: "badminton",
  courts: [courtOne, courtTwo],
  slots: [
    slot("c2", 19, true),
    slot("c1", 19, true),
    slot("c1", 20, false),
    slot("c2", 20, true),
    slot("c1", 9, false),
    slot("c2", 9, false),
  ],
};

describe("hourRows", () => {
  const rows = hourRows(availability, new Date("2026-11-20T10:00:00+07:00"));

  it("orders hours and lists free courts in court order", () => {
    expect(rows.map((row) => row.start.slice(11, 13))).toEqual(["09", "19", "20"]);
    expect(rows[1].free.map((f) => f.court.name)).toEqual(["Lapangan 1", "Lapangan 2"]);
  });

  it("marks hours that have started and leaves full hours empty", () => {
    expect(rows[0].past).toBe(true);
    expect(rows[2].free.map((f) => f.court.name)).toEqual(["Lapangan 2"]);
  });
});

describe("nextHourFree", () => {
  it("allows two hours only when the same court is free for the next hour", () => {
    expect(nextHourFree(availability, slot("c2", 19, true))).toBe(true);
    expect(nextHourFree(availability, slot("c1", 19, true))).toBe(false);
  });
});

describe("deposit", () => {
  it("is half the price rounded up to a thousand, as the API charges", () => {
    expect(deposit(80000)).toBe(40000);
    expect(deposit(75000)).toBe(38000);
    expect(deposit(140000)).toBe(70000);
  });
});
