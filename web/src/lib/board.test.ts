import { describe, expect, it } from "vitest";

import type { OwnerBooking } from "@/api/client";
import {
  canMove,
  conflictWith,
  moveTo,
  placementProblem,
  rowCount,
  rowLabel,
  rowOf,
  rowStart,
} from "./board";

function booking(
  id: string,
  courtId: string,
  start: string,
  end: string,
  status: OwnerBooking["status"] = "confirmed",
): OwnerBooking {
  return {
    id,
    courtId,
    reference: "K7M2QX9P",
    courtName: "Lapangan 1",
    sport: "badminton",
    start: `2026-11-20T${start}:00+07:00`,
    end: `2026-11-20T${end}:00+07:00`,
    status,
    priceIdr: 80000,
    depositIdr: 40000,
    customerName: "Rina Marlina",
    customerPhone: "+6281234567890",
  };
}

describe("board rows", () => {
  it("has a row per half hour from 07.00 to 23.00", () => {
    expect(rowCount).toBe(32);
    expect(rowLabel(0)).toBe("07.00");
    expect(rowLabel(25)).toBe("19.30");
  });

  it("finds the row of a time given in UTC or WIB", () => {
    expect(rowOf("2026-11-20T19:30:00+07:00")).toBe(25);
    expect(rowOf("2026-11-20T12:30:00Z")).toBe(25);
    expect(rowStart("2026-11-20", 25)).toBe("2026-11-20T19:30:00+07:00");
  });
});

describe("moveTo", () => {
  const twoHours = booking("b1", "c1", "19:00", "21:00");

  it("keeps the booking's length on the new court", () => {
    expect(moveTo(twoHours, "2026-11-20", "c2", 26)).toEqual({
      courtId: "c2",
      start: "2026-11-20T20:00:00+07:00",
      end: "2026-11-20T22:00:00+07:00",
    });
  });

  it("refuses a move that would run past closing", () => {
    expect(moveTo(twoHours, "2026-11-20", "c1", 29)).toBeNull();
  });
});

describe("conflictWith", () => {
  const evening = [
    booking("b1", "c1", "19:00", "20:00"),
    booking("b2", "c1", "20:00", "21:00", "held"),
    booking("b3", "c2", "19:00", "20:00", "no_show"),
  ];

  it("finds an overlapping booking on the same court, held ones included", () => {
    const placement = {
      courtId: "c1",
      start: "2026-11-20T20:30:00+07:00",
      end: "2026-11-20T21:30:00+07:00",
    };
    expect(conflictWith(evening, placement)?.id).toBe("b2");
  });

  it("lets bookings touch, ignores the moved booking itself and no-shows", () => {
    const touching = {
      courtId: "c1",
      start: "2026-11-20T21:00:00+07:00",
      end: "2026-11-20T22:00:00+07:00",
    };
    const overItself = {
      courtId: "c1",
      start: "2026-11-20T19:00:00+07:00",
      end: "2026-11-20T20:00:00+07:00",
    };
    const overNoShow = { ...overItself, courtId: "c2" };
    expect(conflictWith(evening, touching)).toBeUndefined();
    expect(conflictWith(evening, overItself, "b1")).toBeUndefined();
    expect(conflictWith(evening, overNoShow)).toBeUndefined();
  });
});

describe("canMove", () => {
  const now = new Date("2026-11-20T19:30:00+07:00");

  it("allows confirmed bookings and walk-ins that are not over", () => {
    expect(canMove(booking("b1", "c1", "19:00", "20:00"), now)).toBe(true);
    expect(canMove(booking("b1", "c1", "19:00", "20:00", "walk_in"), now)).toBe(true);
  });

  it("refuses held bookings, no-shows and bookings that have ended", () => {
    expect(canMove(booking("b1", "c1", "20:00", "21:00", "held"), now)).toBe(false);
    expect(canMove(booking("b1", "c1", "19:00", "20:00", "no_show"), now)).toBe(false);
    expect(canMove(booking("b1", "c1", "18:00", "19:00"), now)).toBe(false);
  });
});

describe("placementProblem", () => {
  const evening = [booking("b1", "c1", "19:00", "20:00")];
  const now = new Date("2026-11-20T18:10:00+07:00");
  const at = (start: string, end: string) => ({
    courtId: "c1",
    start: `2026-11-20T${start}:00+07:00`,
    end: `2026-11-20T${end}:00+07:00`,
  });

  it("names the booking in the way", () => {
    expect(placementProblem(evening, at("19:30", "20:30"), now)).toBe(
      "Bentrok dengan Rina Marlina, 19.00–20.00.",
    );
  });

  it("refuses times that are over or past closing", () => {
    expect(placementProblem(evening, at("17:00", "18:00"), now)).toBe("Jam itu sudah lewat.");
    expect(placementProblem(evening, null, now)).toBe("Waktunya melewati jam tutup 23.00.");
  });

  it("accepts a free time that has not ended", () => {
    expect(placementProblem(evening, at("18:00", "19:00"), now)).toBeUndefined();
  });
});
