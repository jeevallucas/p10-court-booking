import {
  api,
  type OwnerBooking,
  type OwnerDay,
  type Problem,
  type WalkInRequest,
} from "@/api/client";
import type { Placement } from "@/lib/board";
import { problemMessage } from "@/lib/problems";

/** A failed owner call, with the API's problem code when there was one. */
export class OwnerApiError extends Error {
  constructor(
    message: string,
    readonly code?: Problem["code"],
  ) {
    super(message);
  }

  get sessionEnded(): boolean {
    return this.code === "unauthorized";
  }
}

const sessionEndedMessage = "Sesi pengelola sudah berakhir. Masuk lagi untuk melanjutkan.";

function failure(body: unknown, response: Response, slotTakenMessage?: string): OwnerApiError {
  if (response.status === 401) {
    return new OwnerApiError(sessionEndedMessage, "unauthorized");
  }
  const code =
    typeof body === "object" && body !== null && "code" in body
      ? (body.code as Problem["code"])
      : undefined;
  if (code === "slot_taken" && slotTakenMessage) {
    return new OwnerApiError(slotTakenMessage, code);
  }
  return new OwnerApiError(problemMessage(body), code);
}

export async function fetchOwnerDay(date: string): Promise<OwnerDay> {
  const { data, error, response } = await api.GET("/v1/owner/bookings", {
    params: { query: { date } },
  });
  if (!data) {
    throw failure(error, response);
  }
  return data;
}

export async function rescheduleBooking(id: string, placement: Placement): Promise<OwnerBooking> {
  const { data, error, response } = await api.PATCH("/v1/owner/bookings/{id}", {
    params: { path: { id } },
    body: placement,
  });
  if (!data) {
    throw failure(
      error,
      response,
      "Jam itu baru saja terisi booking lain, jadi booking dikembalikan ke tempat semula.",
    );
  }
  return data;
}

export type OwnerStatus = "confirmed" | "cancelled" | "no_show";

export async function changeBookingStatus(id: string, status: OwnerStatus): Promise<OwnerBooking> {
  const { data, error, response } = await api.POST("/v1/owner/bookings/{id}/status", {
    params: { path: { id } },
    body: { status },
  });
  if (!data) {
    throw failure(error, response);
  }
  return data;
}

export async function createWalkIn(request: WalkInRequest): Promise<OwnerBooking> {
  const { data, error, response } = await api.POST("/v1/owner/walk-ins", { body: request });
  if (!data) {
    throw failure(
      error,
      response,
      "Lapangan itu sudah terisi pada jam tersebut. Pilih jam atau lapangan lain.",
    );
  }
  return data;
}

export async function logOut(): Promise<void> {
  // An already-ended session still counts as logged out.
  await api.DELETE("/v1/owner/session");
}
