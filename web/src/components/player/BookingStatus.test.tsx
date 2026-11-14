import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Booking } from "@/api/client";
import { answerWith, json } from "@/test/fakeApi";
import { renderWithQueries } from "@/test/render";

import { BookingStatus } from "./BookingStatus";

const booking: Booking = {
  reference: "K7M2QX",
  courtName: "Lapangan 2",
  sport: "badminton",
  start: "2026-11-20T19:00:00+07:00",
  end: "2026-11-20T20:00:00+07:00",
  status: "confirmed",
  priceIdr: 80000,
  depositIdr: 40000,
};

describe("BookingStatus", () => {
  it("shows what is left to pay at the counter once the deposit is in", async () => {
    answerWith(() => json(200, booking));
    renderWithQueries(<BookingStatus reference="K7M2QX" />);

    expect(await screen.findByText("Booking terkonfirmasi")).toBeTruthy();
    expect(screen.getByText(/Sisa Rp40\.000 dibayar di kasir/)).toBeTruthy();
  });

  it("keeps the payment link while the slot is held", async () => {
    const heldUntil = new Date(Date.now() + 5 * 60_000).toISOString();
    answerWith(() =>
      json(200, {
        ...booking,
        status: "held",
        holdExpiresAt: heldUntil,
        paymentUrl: "/fakepay/K7M2QX",
      }),
    );
    renderWithQueries(<BookingStatus reference="K7M2QX" />);

    const pay = await screen.findByRole("link", { name: "Lanjutkan pembayaran" });
    expect(pay.getAttribute("href")).toBe("/fakepay/K7M2QX");
    expect(screen.getByLabelText(/menit/).textContent).toMatch(/^[45]\.\d\d$/);
  });

  it("says when a booking code does not exist", async () => {
    answerWith(() => json(404, { code: "not_found", detail: "no booking K7M2QX" }));
    renderWithQueries(<BookingStatus reference="K7M2QX" />);

    expect(
      await screen.findByRole("heading", { name: "Booking K7M2QX tidak ditemukan" }),
    ).toBeTruthy();
  });

  it("offers a retry when the API is down behind the proxy", async () => {
    answerWith(() => new Response("<html>502 Bad Gateway</html>", { status: 502 }));
    renderWithQueries(<BookingStatus reference="K7M2QX" />);

    expect((await screen.findByRole("alert")).textContent).toMatch(/tidak bisa dihubungi/);
    expect(screen.getByRole("button", { name: "Muat ulang status" })).toBeTruthy();
  });
});
