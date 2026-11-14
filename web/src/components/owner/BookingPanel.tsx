"use client";

import { useState, type FormEvent } from "react";

import type { Court, OwnerBooking } from "@/api/client";
import {
  canMove,
  moveTo,
  placementProblem,
  rowCount,
  rowLabel,
  rowOf,
  rowsSpanned,
  statusLabels,
  type Placement,
} from "@/lib/board";
import { longDate, rupiah, timeLabel } from "@/lib/format";

import styles from "./OwnerPanel.module.css";
import type { OwnerStatus } from "./ownerApi";

interface Props {
  booking: OwnerBooking;
  courts: Court[];
  bookings: OwnerBooking[];
  day: string;
  now: Date;
  busy: boolean;
  onMove: (placement: Placement) => void;
  onStatus: (to: OwnerStatus) => void;
  onClose: () => void;
}

export function BookingPanel({
  booking,
  courts,
  bookings,
  day,
  now,
  busy,
  onMove,
  onStatus,
  onClose,
}: Props) {
  const [confirmingCancel, setConfirmingCancel] = useState(false);
  const started = Date.parse(booking.start) <= now.getTime();
  const occupiesCourt = booking.status !== "no_show";
  const dueAtDesk = booking.priceIdr - (booking.status === "walk_in" ? 0 : booking.depositIdr);

  return (
    <div className={styles.panel}>
      <div className={styles.head}>
        <p className={styles.state} data-status={booking.status}>
          {statusLabels[booking.status]}
        </p>
        <button type="button" className={styles.close} onClick={onClose}>
          Tutup
        </button>
      </div>
      <h2>{booking.customerName}</h2>
      <p className={styles.when}>
        {booking.courtName}, {longDate(booking.start)}
        <br />
        <span className="time">
          {timeLabel(booking.start)}–{timeLabel(booking.end)}
        </span>{" "}
        WIB
      </p>

      <dl className={styles.details}>
        <div>
          <dt>Kode booking</dt>
          <dd>{booking.reference}</dd>
        </div>
        <div>
          <dt>WhatsApp</dt>
          <dd>{booking.customerPhone || "Tidak dicatat"}</dd>
        </div>
        <div>
          <dt>Harga</dt>
          <dd>{rupiah(booking.priceIdr)}</dd>
        </div>
        {booking.status !== "walk_in" && (
          <div>
            <dt>{booking.status === "held" ? "DP belum masuk" : "DP masuk"}</dt>
            <dd>{rupiah(booking.depositIdr)}</dd>
          </div>
        )}
        {occupiesCourt && (
          <div>
            <dt>Sisa di kasir</dt>
            <dd>{rupiah(dueAtDesk)}</dd>
          </div>
        )}
      </dl>

      {booking.status === "held" && (
        <div className={styles.section}>
          <p>
            Pemain belum membayar DP online. Kalau DP dibayar di kasir, tandai di sini; kalau tidak,
            slot dilepas otomatis pukul{" "}
            {booking.holdExpiresAt ? timeLabel(booking.holdExpiresAt) : "-"}.
          </p>
          <button
            type="button"
            className={styles.primary}
            disabled={busy}
            onClick={() => onStatus("confirmed")}
          >
            Tandai DP dibayar di kasir
          </button>
        </div>
      )}

      {canMove(booking, now) && (
        <MoveForm
          booking={booking}
          courts={courts}
          bookings={bookings}
          day={day}
          now={now}
          busy={busy}
          onMove={onMove}
        />
      )}

      {(booking.status === "confirmed" || booking.status === "walk_in") && (
        <div className={styles.section}>
          {started ? (
            <button
              type="button"
              className={styles.secondary}
              disabled={busy}
              onClick={() => onStatus("no_show")}
            >
              Tandai tidak datang
            </button>
          ) : (
            <p className={styles.hint}>
              Tidak datang bisa ditandai mulai pukul {timeLabel(booking.start)}.
            </p>
          )}
        </div>
      )}

      {booking.status === "no_show" && (
        <p className={styles.section}>
          Ditandai tidak datang. Lapangannya bisa dipakai booking lain pada jam ini.
        </p>
      )}

      {occupiesCourt && (
        <div className={styles.section}>
          {confirmingCancel ? (
            <div className={styles.confirm} role="group" aria-label="Konfirmasi pembatalan">
              <p>
                Batalkan booking {booking.customerName}? Slotnya kosong lagi dan pembatalan tidak
                bisa diurungkan.
              </p>
              <button
                type="button"
                className={styles.danger}
                disabled={busy}
                onClick={() => onStatus("cancelled")}
              >
                Ya, batalkan booking
              </button>
              <button
                type="button"
                className={styles.secondary}
                onClick={() => setConfirmingCancel(false)}
              >
                Jangan batalkan
              </button>
            </div>
          ) : (
            <button
              type="button"
              className={styles.secondary}
              onClick={() => setConfirmingCancel(true)}
            >
              Batalkan booking
            </button>
          )}
        </div>
      )}
    </div>
  );
}

/** The keyboard and touch way to do what dragging a block does. */
function MoveForm({
  booking,
  courts,
  bookings,
  day,
  now,
  busy,
  onMove,
}: Omit<Props, "onStatus" | "onClose">) {
  const [courtId, setCourtId] = useState(booking.courtId);
  const [row, setRow] = useState(rowOf(booking.start));
  const length = rowsSpanned(booking);
  const placement = moveTo(booking, day, courtId, row);
  const problem = placementProblem(bookings, placement, now, booking.id);
  const unchanged = courtId === booking.courtId && row === rowOf(booking.start);

  function submit(event: FormEvent) {
    event.preventDefault();
    if (placement && !problem && !unchanged) {
      onMove(placement);
    }
  }

  return (
    <form className={styles.section} onSubmit={submit}>
      <h3>Pindahkan</h3>
      <div className={styles.row}>
        <label className={styles.field}>
          Lapangan
          <select value={courtId} onChange={(event) => setCourtId(event.target.value)}>
            {courts
              .filter((court) => court.sport === booking.sport)
              .map((court) => (
                <option key={court.id} value={court.id}>
                  {court.name}
                </option>
              ))}
          </select>
        </label>
        <label className={styles.field}>
          Mulai
          <select value={row} onChange={(event) => setRow(Number(event.target.value))}>
            {Array.from({ length: rowCount - length + 1 }, (_, option) => (
              <option key={option} value={option}>
                {rowLabel(option)}
              </option>
            ))}
          </select>
        </label>
      </div>
      {!unchanged && problem && (
        <p className={styles.problem} role="status">
          {problem}
        </p>
      )}
      <button
        type="submit"
        className={styles.primary}
        disabled={busy || unchanged || Boolean(problem)}
      >
        Pindahkan booking
      </button>
    </form>
  );
}
