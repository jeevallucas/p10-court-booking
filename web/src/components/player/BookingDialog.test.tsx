import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Availability } from "@/api/client";
import { answerWith, json } from "@/test/fakeApi";
import { renderWithQueries } from "@/test/render";

import { BookingDialog } from "./BookingDialog";

const court = { id: "c1", name: "Lapangan 1", sport: "badminton" as const };
const seven = {
  courtId: "c1",
  start: "2026-11-20T19:00:00+07:00",
  end: "2026-11-20T20:00:00+07:00",
  priceIdr: 80000,
  available: true,
};
const eight = { ...seven, start: seven.end, end: "2026-11-20T21:00:00+07:00", available: false };
const availability: Availability = {
  date: "2026-11-20",
  sport: "badminton",
  courts: [court],
  slots: [seven, eight],
};

function openDialog(onSlotTaken = () => {}) {
  renderWithQueries(
    <BookingDialog
      chosen={{ court, slot: seven }}
      availability={availability}
      onClose={() => {}}
      onSlotTaken={onSlotTaken}
    />,
  );
}

function fill(name: string, phone: string) {
  fireEvent.change(screen.getByLabelText("Nama pemesan"), { target: { value: name } });
  fireEvent.change(screen.getByLabelText(/Nomor WhatsApp/), { target: { value: phone } });
  fireEvent.click(screen.getByRole("button", { name: "Bayar DP Rp40.000" }));
}

describe("BookingDialog", () => {
  it("asks for a name and an Indonesian WhatsApp number before holding the slot", () => {
    openDialog();
    fill("A", "12345");

    expect(screen.getByText("Tulis nama pemesan, minimal 2 huruf.")).toBeTruthy();
    expect(screen.getByText("Tulis nomor WhatsApp yang dimulai 08, 628, atau +628.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Bayar DP Rp40.000" })).toBeTruthy();
  });

  it("offers two hours only when the same court's next hour is free", () => {
    openDialog();
    const twoHours = screen.getByLabelText(/2 jam/) as HTMLInputElement;

    expect(twoHours.disabled).toBe(true);
    expect(screen.getByText(/jam berikutnya sudah dipesan/)).toBeTruthy();
  });

  it("tells the player when someone else took the slot and refreshes the schedule", async () => {
    let refreshed = false;
    answerWith(() => json(409, { code: "slot_taken", detail: "slot is already booked" }));
    openDialog(() => {
      refreshed = true;
    });
    fill("Rina", "0812 3456 7890");

    expect((await screen.findByRole("alert")).textContent).toMatch(/baru saja dipesan orang lain/);
    expect(refreshed).toBe(true);
  });

  it("says the server cannot be reached when the connection drops", async () => {
    answerWith(() => Promise.reject(new TypeError("Failed to fetch")));
    openDialog();
    fill("Rina", "081234567890");

    expect((await screen.findByRole("alert")).textContent).toMatch(/tidak bisa dihubungi/);
  });
});
