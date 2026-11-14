import type { Court, OwnerBooking, OwnerDay } from "@/api/client";

export const badmintonOne: Court = { id: "c1", name: "Lapangan 1", sport: "badminton" };
export const badmintonTwo: Court = { id: "c2", name: "Lapangan 2", sport: "badminton" };
export const futsalA: Court = { id: "c5", name: "Futsal A", sport: "futsal" };

export function ownerBooking(overrides: Partial<OwnerBooking> = {}): OwnerBooking {
  return {
    id: "b1",
    courtId: "c1",
    reference: "K7M2QX9P",
    courtName: "Lapangan 1",
    sport: "badminton",
    start: "2026-11-20T19:00:00+07:00",
    end: "2026-11-20T20:00:00+07:00",
    status: "confirmed",
    priceIdr: 80000,
    depositIdr: 40000,
    customerName: "Rina Marlina",
    customerPhone: "+6281234567890",
    ...overrides,
  };
}

export function ownerDay(bookings: OwnerBooking[]): OwnerDay {
  return { date: "2026-11-20", courts: [badmintonOne, badmintonTwo, futsalA], bookings };
}
