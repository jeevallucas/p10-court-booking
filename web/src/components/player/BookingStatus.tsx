"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useEffect, useState } from "react";

import { api, type Booking } from "@/api/client";
import { SlowServerNote } from "@/components/SlowServerNote";
import { longDate, rupiah, timeLabel } from "@/lib/format";
import { problemMessage } from "@/lib/problems";

import styles from "./BookingStatus.module.css";

export function BookingStatus({ reference }: { reference: string }) {
  const booking = useQuery({
    queryKey: ["booking", reference],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/v1/bookings/{reference}", {
        params: { path: { reference } },
      });
      // A mistyped code fails the API's format check (422) and can no more exist than an
      // unknown one (404); both tell the player to check the code.
      if (response?.status === 404 || response?.status === 422) {
        return null;
      }
      if (!data) {
        throw new Error(problemMessage(error));
      }
      return data;
    },
    // While the deposit is pending, the payment notification can land at any moment.
    refetchInterval: (query) => (query.state.data?.status === "held" ? 3000 : false),
  });

  return (
    <main className={styles.page}>
      <Link href="/" className={styles.venue}>
        Arena Kotabaru
      </Link>
      {booking.isPending && (
        <>
          <p aria-live="polite">Memuat booking {reference}…</p>
          <SlowServerNote />
        </>
      )}
      {booking.isError && (
        <div role="alert" className={styles.panel}>
          <p>Status booking gagal dimuat. {booking.error.message}</p>
          <button type="button" onClick={() => booking.refetch()}>
            Muat ulang status
          </button>
        </div>
      )}
      {booking.data === null && (
        <div className={styles.panel}>
          <h1>Booking {reference} tidak ditemukan</h1>
          <p>Periksa lagi kode booking dari pesan konfirmasi, atau pesan lapangan baru.</p>
          <Link href="/" className={styles.action}>
            Lihat jam kosong
          </Link>
        </div>
      )}
      {booking.data && <Summary booking={booking.data} />}
    </main>
  );
}

const headings: Record<Booking["status"], string> = {
  held: "Selesaikan pembayaran DP",
  confirmed: "Booking terkonfirmasi",
  walk_in: "Booking terkonfirmasi",
  expired: "Waktu pembayaran habis",
  cancelled: "Booking dibatalkan",
  no_show: "Booking ditandai tidak datang",
};

function Summary({ booking }: { booking: Booking }) {
  const confirmed = booking.status === "confirmed" || booking.status === "walk_in";
  return (
    <article className={styles.panel} aria-live="polite">
      <p className={styles.state} data-status={booking.status}>
        {headings[booking.status]}
      </p>
      <h1>{booking.courtName}</h1>
      <p>{longDate(booking.start)}</p>
      <p className={styles.when}>
        <span className={`time ${styles.time}`}>
          {timeLabel(booking.start)}–{timeLabel(booking.end)}
        </span>{" "}
        WIB
      </p>
      <dl className={styles.details}>
        <div>
          <dt>Kode booking</dt>
          <dd className={styles.reference}>{booking.reference}</dd>
        </div>
        <div>
          <dt>Harga</dt>
          <dd>{rupiah(booking.priceIdr)}</dd>
        </div>
        <div>
          <dt>{confirmed ? "DP dibayar" : "DP"}</dt>
          <dd>{rupiah(booking.depositIdr)}</dd>
        </div>
      </dl>

      {booking.status === "held" && booking.holdExpiresAt && (
        <HeldActions paymentUrl={booking.paymentUrl} expiresAt={booking.holdExpiresAt} />
      )}
      {confirmed && (
        <p>
          Sisa {rupiah(booking.priceIdr - booking.depositIdr)} dibayar di kasir. Tunjukkan kode
          booking saat datang. Konfirmasi juga dikirim ke WhatsApp kamu.
        </p>
      )}
      {(booking.status === "expired" || booking.status === "cancelled") && (
        <>
          <p>
            Slot ini sudah dilepas. Kalau DP sempat terpotong, pengelola venue akan menghubungimu.
          </p>
          <Link href="/" className={styles.action}>
            Pilih jam lain
          </Link>
        </>
      )}
    </article>
  );
}

function HeldActions({ paymentUrl, expiresAt }: { paymentUrl?: string | null; expiresAt: string }) {
  const remaining = useCountdown(expiresAt);
  const minutes = Math.floor(remaining / 60);
  const seconds = String(remaining % 60).padStart(2, "0");
  return (
    <>
      <p>
        Slot ditahan untukmu selama{" "}
        <strong className="time" aria-label={`${minutes} menit ${seconds} detik`}>
          {minutes}.{seconds}
        </strong>{" "}
        lagi. Halaman ini diperbarui otomatis setelah pembayaran masuk.
      </p>
      {paymentUrl && (
        <a href={paymentUrl} className={styles.action}>
          Lanjutkan pembayaran
        </a>
      )}
    </>
  );
}

function useCountdown(until: string): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  return Math.max(0, Math.round((new Date(until).getTime() - now) / 1000));
}
