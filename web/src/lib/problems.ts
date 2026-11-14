import type { Problem } from "@/api/client";

// The API's problem codes, told to the player in plain Indonesian with what to do next.
const messages: Record<Problem["code"], string> = {
  slot_taken: "Slot ini baru saja dipesan orang lain. Pilih jam atau lapangan lain.",
  invalid_request:
    "Data pemesanan belum lengkap atau tidak sesuai. Periksa nama dan nomor WhatsApp.",
  not_found: "Booking tidak ditemukan. Periksa kode booking kamu.",
  rate_limited: "Terlalu banyak percobaan dari perangkat ini. Tunggu satu menit, lalu coba lagi.",
  unauthorized: "Sesi kamu sudah berakhir. Masuk lagi.",
  invalid_transition: "Status booking ini sudah berubah. Muat ulang halaman.",
  outside_opening_hours: "Jam itu di luar jam buka (07.00–23.00 WIB). Pilih jam lain.",
  in_the_past: "Jam itu sudah lewat. Pilih jam yang belum dimulai.",
  invalid_signature: "Notifikasi pembayaran tidak sah.",
  amount_mismatch: "Jumlah pembayaran tidak sesuai DP.",
};

export const unreachableMessage =
  "Server booking tidak bisa dihubungi. Periksa koneksi internet, lalu coba lagi.";

/**
 * Turns an error body into a message for the player. Anything that is not one of the API's
 * problems (no response, or an HTML error page from a proxy while the API is down) means the
 * booking server could not answer.
 */
export function problemMessage(body: unknown): string {
  if (typeof body === "object" && body !== null && "code" in body) {
    const message = messages[body.code as Problem["code"]];
    if (message) {
      return message;
    }
  }
  return unreachableMessage;
}
