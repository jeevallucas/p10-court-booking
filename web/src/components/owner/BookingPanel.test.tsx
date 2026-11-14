import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { OwnerBooking } from "@/api/client";
import { badmintonOne, badmintonTwo, ownerBooking } from "@/test/ownerFixtures";

import { BookingPanel } from "./BookingPanel";
import type { OwnerStatus } from "./ownerApi";

function openPanel(booking: OwnerBooking, now: Date) {
  const changes: OwnerStatus[] = [];
  render(
    <BookingPanel
      booking={booking}
      courts={[badmintonOne, badmintonTwo]}
      bookings={[booking]}
      day="2026-11-20"
      now={now}
      busy={false}
      onMove={() => {}}
      onStatus={(to) => changes.push(to)}
      onClose={() => {}}
    />,
  );
  return changes;
}

describe("BookingPanel", () => {
  it("asks before cancelling, because a cancellation cannot be undone", () => {
    const changes = openPanel(ownerBooking(), new Date("2026-11-20T10:00:00+07:00"));

    fireEvent.click(screen.getByRole("button", { name: "Batalkan booking" }));
    expect(screen.getByText(/tidak bisa diurungkan/)).toBeTruthy();
    expect(changes).toEqual([]);

    fireEvent.click(screen.getByRole("button", { name: "Ya, batalkan booking" }));
    expect(changes).toEqual(["cancelled"]);
  });

  it("offers no-show only once the booking has started", () => {
    openPanel(ownerBooking(), new Date("2026-11-20T18:00:00+07:00"));

    expect(screen.queryByRole("button", { name: "Tandai tidak datang" })).toBeNull();
    expect(screen.getByText("Tidak datang bisa ditandai mulai pukul 19.00.")).toBeTruthy();
  });

  it("lets a held booking be confirmed at the desk but not moved", () => {
    const changes = openPanel(
      ownerBooking({ status: "held", holdExpiresAt: "2026-11-20T10:08:00+07:00" }),
      new Date("2026-11-20T10:00:00+07:00"),
    );

    expect(screen.queryByRole("button", { name: "Pindahkan booking" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Tandai DP dibayar di kasir" }));
    expect(changes).toEqual(["confirmed"]);
  });
});
