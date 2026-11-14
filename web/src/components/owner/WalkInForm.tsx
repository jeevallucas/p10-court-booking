"use client";

import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import type { Court, OwnerBooking } from "@/api/client";
import { placementProblem, rowCount, rowLabel, rowStart, slotMinutes } from "@/lib/board";
import { compactPhone, isIndonesianMobile, phoneHint } from "@/lib/phone";

import { createWalkIn, OwnerApiError } from "./ownerApi";
import styles from "./OwnerPanel.module.css";

const durations = [
  { rows: 1, label: "30 menit" },
  { rows: 2, label: "1 jam" },
  { rows: 3, label: "1,5 jam" },
  { rows: 4, label: "2 jam" },
  { rows: 6, label: "3 jam" },
];

interface Props {
  courts: Court[];
  bookings: OwnerBooking[];
  day: string;
  now: Date;
  initialCourtId?: string;
  initialRow?: number;
  onCreated: (booking: OwnerBooking) => void;
  onSessionEnded: () => void;
  onClose: () => void;
}

export function WalkInForm({
  courts,
  bookings,
  day,
  now,
  initialCourtId,
  initialRow,
  onCreated,
  onSessionEnded,
  onClose,
}: Props) {
  const [row, setRow] = useState(initialRow ?? firstOpenRow(day, now));
  const [rows, setRows] = useState(2);
  // Opened from the toolbar, start on the first court that is free for the next hour.
  const [courtId, setCourtId] = useState(
    () =>
      initialCourtId ??
      courts.find(
        (court) =>
          !placementProblem(
            bookings,
            { courtId: court.id, start: rowStart(day, row), end: rowStart(day, row + rows) },
            now,
          ),
      )?.id ??
      courts[0]?.id ??
      "",
  );
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [touched, setTouched] = useState(false);

  const placement =
    row + rows <= rowCount
      ? { courtId, start: rowStart(day, row), end: rowStart(day, row + rows) }
      : null;
  const problem = placementProblem(bookings, placement, now);
  const nameError = name.trim().length < 2 ? "Tulis nama pelanggan, minimal 2 huruf." : "";
  const phoneError = phone.trim() !== "" && !isIndonesianMobile(phone) ? phoneHint : "";

  const save = useMutation({
    mutationFn: () =>
      createWalkIn({
        courtId,
        start: rowStart(day, row),
        end: rowStart(day, row + rows),
        customerName: name.trim(),
        ...(phone.trim() !== "" && { customerPhone: compactPhone(phone) }),
      }),
    onSuccess: onCreated,
    onError: (error) => {
      if (error instanceof OwnerApiError && error.sessionEnded) {
        onSessionEnded();
      }
    },
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    setTouched(true);
    if (!problem && !nameError && !phoneError) {
      save.mutate();
    }
  }

  return (
    <form className={styles.panel} onSubmit={submit} noValidate>
      <div className={styles.head}>
        <h2>Tambah walk-in</h2>
        <button type="button" className={styles.close} onClick={onClose}>
          Tutup
        </button>
      </div>
      <p className={styles.hint}>Untuk pelanggan di meja depan; dibayar penuh di kasir.</p>

      <div className={styles.row}>
        <label className={styles.field}>
          Lapangan
          <select value={courtId} onChange={(event) => setCourtId(event.target.value)}>
            {courts.map((court) => (
              <option key={court.id} value={court.id}>
                {court.name}
              </option>
            ))}
          </select>
        </label>
        <label className={styles.field}>
          Mulai
          <select value={row} onChange={(event) => setRow(Number(event.target.value))}>
            {Array.from({ length: rowCount }, (_, option) => (
              <option key={option} value={option}>
                {rowLabel(option)}
              </option>
            ))}
          </select>
        </label>
        <label className={styles.field}>
          Durasi
          <select value={rows} onChange={(event) => setRows(Number(event.target.value))}>
            {durations.map((duration) => (
              <option key={duration.rows} value={duration.rows}>
                {duration.label}
              </option>
            ))}
          </select>
        </label>
      </div>
      {problem && (
        <p className={styles.problem} role="status">
          {problem}
        </p>
      )}

      <label className={styles.field}>
        Nama pelanggan
        <input
          name="name"
          maxLength={60}
          value={name}
          onChange={(event) => setName(event.target.value)}
          aria-invalid={touched && nameError !== ""}
          aria-describedby="walk-in-name-error"
        />
        <span id="walk-in-name-error" className={styles.error}>
          {touched && nameError}
        </span>
      </label>
      <label className={styles.field}>
        Nomor WhatsApp (opsional)
        <input
          name="phone"
          type="tel"
          inputMode="tel"
          value={phone}
          onChange={(event) => setPhone(event.target.value)}
          aria-invalid={touched && phoneError !== ""}
          aria-describedby="walk-in-phone-error"
        />
        <span id="walk-in-phone-error" className={styles.error}>
          {touched && phoneError}
        </span>
      </label>

      {save.isError && (
        <p className={styles.problem} role="alert">
          {save.error.message}
        </p>
      )}
      <button
        type="submit"
        className={styles.primary}
        disabled={save.isPending || Boolean(problem)}
      >
        {save.isPending ? "Menyimpan…" : "Simpan walk-in"}
      </button>
    </form>
  );
}

/** Today: the half hour that is running now; another day: opening time. */
function firstOpenRow(day: string, now: Date): number {
  const elapsed = (now.getTime() - Date.parse(rowStart(day, 0))) / (slotMinutes * 60_000);
  return Math.min(rowCount - 1, Math.max(0, Math.floor(elapsed)));
}
