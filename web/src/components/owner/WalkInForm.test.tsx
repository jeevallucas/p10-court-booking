import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { answerWith, json } from "@/test/fakeApi";
import { badmintonOne, badmintonTwo, ownerBooking } from "@/test/ownerFixtures";
import { renderWithQueries } from "@/test/render";

import { WalkInForm } from "./WalkInForm";

const now = new Date("2026-11-20T10:00:00+07:00");

function openForm(row: number) {
  renderWithQueries(
    <WalkInForm
      courts={[badmintonOne, badmintonTwo]}
      bookings={[ownerBooking()]}
      day="2026-11-20"
      now={now}
      initialCourtId="c1"
      initialRow={row}
      onCreated={() => {}}
      onSessionEnded={() => {}}
      onClose={() => {}}
    />,
  );
}

function saveButton(): HTMLButtonElement {
  return screen.getByRole("button", { name: "Simpan walk-in" });
}

describe("WalkInForm", () => {
  it("names the booking in the way before anything is sent", () => {
    openForm(24);

    expect(screen.getByText("Bentrok dengan Rina Marlina, 19.00–20.00.")).toBeTruthy();
    expect(saveButton().disabled).toBe(true);
  });

  it("asks for a name and accepts a walk-in without a phone number", () => {
    openForm(20);
    fireEvent.click(saveButton());

    expect(screen.getByText("Tulis nama pelanggan, minimal 2 huruf.")).toBeTruthy();
    expect(screen.queryByText(/nomor WhatsApp yang dimulai/)).toBeNull();
  });

  it("explains a conflict the API found after another booking landed first", async () => {
    answerWith(() => json(409, { code: "slot_taken", detail: "overlaps another booking" }));
    openForm(20);
    fireEvent.change(screen.getByLabelText("Nama pelanggan"), {
      target: { value: "Joko Susilo" },
    });
    fireEvent.click(saveButton());

    expect((await screen.findByRole("alert")).textContent).toMatch(
      /sudah terisi pada jam tersebut/,
    );
  });
});
