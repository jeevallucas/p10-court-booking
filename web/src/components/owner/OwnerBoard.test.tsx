import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { answerWith, json } from "@/test/fakeApi";
import { ownerBooking, ownerDay } from "@/test/ownerFixtures";
import { renderWithQueries } from "@/test/render";

import { OwnerBoard } from "./OwnerBoard";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: () => {}, replace: () => {} }) }));

function column(courtId: string): HTMLElement {
  const element = document.querySelector<HTMLElement>(`[data-court-id="${courtId}"]`);
  if (!element) {
    throw new Error(`no column for court ${courtId}`);
  }
  return element;
}

describe("OwnerBoard", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-11-20T10:00:00+07:00"));
  });
  afterEach(() => vi.useRealTimers());

  it("puts a moved booking back and says why when the API reports a conflict", async () => {
    let dayLoads = 0;
    answerWith((request) => {
      if (request.method === "PATCH") {
        return json(409, { code: "slot_taken", detail: "overlaps another booking" });
      }
      dayLoads += 1;
      // Only the first load answers, so the booking can only return through the rollback.
      return dayLoads === 1 ? json(200, ownerDay([ownerBooking()])) : new Promise(() => {});
    });
    renderWithQueries(<OwnerBoard />);

    fireEvent.click(await screen.findByRole("button", { name: /Rina Marlina/ }));
    fireEvent.change(screen.getByLabelText("Lapangan"), { target: { value: "c2" } });
    fireEvent.click(screen.getByRole("button", { name: "Pindahkan booking" }));

    expect(await screen.findByText(/baru saja terisi booking lain/)).toBeTruthy();
    expect(within(column("c1")).getByText("Rina Marlina")).toBeTruthy();
    expect(within(column("c2")).queryByText("Rina Marlina")).toBeNull();
  });

  it("shows the state of each booking in words, not only in colour", async () => {
    answerWith(() =>
      json(
        200,
        ownerDay([
          ownerBooking({ id: "b1", status: "held", customerName: "Dewi Lestari" }),
          ownerBooking({ id: "b2", courtId: "c2", status: "walk_in", customerName: "Agus" }),
          ownerBooking({ id: "b3", courtId: "c5", status: "no_show", customerName: "Tim Gejayan" }),
        ]),
      ),
    );
    renderWithQueries(<OwnerBoard />);

    const held = await screen.findByRole("button", { name: /Dewi Lestari/ });
    expect(held.textContent).toContain("Menunggu DP");
    expect(screen.getByRole("button", { name: /Agus/ }).textContent).toContain("Walk-in");
    expect(screen.getByRole("button", { name: /Tim Gejayan/ }).textContent).toContain(
      "Tidak datang",
    );
  });

  it("offers a retry when the schedule cannot load", async () => {
    answerWith(() => new Response("<html>502 Bad Gateway</html>", { status: 502 }));
    renderWithQueries(<OwnerBoard />);

    // The board retries a failed load once, about a second later, before showing the error.
    const alert = await screen.findByRole("alert", {}, { timeout: 3000 });
    expect(alert.textContent).toMatch(/tidak bisa dihubungi/);
    expect(screen.getByRole("button", { name: "Muat ulang jadwal" })).toBeTruthy();
  });
});
