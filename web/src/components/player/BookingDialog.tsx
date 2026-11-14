"use client";

import { useMutation } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { api, type Availability } from "@/api/client";
import { longDate, rupiah, timeLabel } from "@/lib/format";
import { compactPhone, isIndonesianMobile, phoneHint } from "@/lib/phone";
import { problemMessage } from "@/lib/problems";
import { deposit, nextHourFree, type FreeCourt } from "@/lib/slots";

import styles from "./BookingDialog.module.css";

interface Props {
  chosen: FreeCourt;
  availability: Availability;
  onClose: () => void;
  onSlotTaken: () => void;
}

export function BookingDialog({ chosen, availability, onClose, onSlotTaken }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const formId = useId();
  const [hours, setHours] = useState<1 | 2>(1);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    return () => element?.close();
  }, []);

  const canBookTwo = nextHourFree(availability, chosen.slot);
  const secondHour = availability.slots.find(
    (slot) => slot.courtId === chosen.slot.courtId && slot.start === chosen.slot.end,
  );
  const price = chosen.slot.priceIdr + (hours === 2 && secondHour ? secondHour.priceIdr : 0);
  const end = hours === 2 && secondHour ? secondHour.end : chosen.slot.end;

  const nameError = name.trim().length < 2 ? "Tulis nama pemesan, minimal 2 huruf." : "";
  const phoneError = isIndonesianMobile(phone) ? "" : phoneHint;

  const hold = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/v1/holds", {
        body: {
          courtId: chosen.court.id,
          start: chosen.slot.start,
          hours,
          customerName: name.trim(),
          customerPhone: compactPhone(phone),
        },
      });
      if (!data) {
        if (error?.code === "slot_taken") {
          onSlotTaken();
        }
        throw new Error(problemMessage(error));
      }
      return data;
    },
    onSuccess: (held) => {
      if (held.paymentUrl) {
        window.location.assign(held.paymentUrl);
      }
    },
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    setTouched(true);
    if (!nameError && !phoneError) {
      hold.mutate();
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby={`${formId}-title`}
      onClose={onClose}
    >
      <form onSubmit={submit} noValidate>
        <h2 id={`${formId}-title`}>Pesan {chosen.court.name}</h2>
        <p className={styles.when}>
          {longDate(chosen.slot.start)}
          <br />
          <span className="time">
            {timeLabel(chosen.slot.start)}–{timeLabel(end)}
          </span>{" "}
          WIB
        </p>

        <fieldset className={styles.duration}>
          <legend>Durasi</legend>
          <label>
            <input type="radio" name="hours" checked={hours === 1} onChange={() => setHours(1)} />1
            jam
          </label>
          <label>
            <input
              type="radio"
              name="hours"
              checked={hours === 2}
              disabled={!canBookTwo}
              onChange={() => setHours(2)}
            />
            2 jam{!canBookTwo && " (jam berikutnya sudah dipesan)"}
          </label>
        </fieldset>

        <dl className={styles.costs}>
          <div>
            <dt>Harga</dt>
            <dd>{rupiah(price)}</dd>
          </div>
          <div>
            <dt>DP sekarang</dt>
            <dd>{rupiah(deposit(price))}</dd>
          </div>
          <div>
            <dt>Sisa di kasir</dt>
            <dd>{rupiah(price - deposit(price))}</dd>
          </div>
        </dl>

        <label className={styles.field}>
          Nama pemesan
          <input
            name="name"
            autoComplete="name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            aria-invalid={touched && nameError !== ""}
            aria-describedby={`${formId}-name-error`}
          />
          <span id={`${formId}-name-error`} className={styles.error}>
            {touched && nameError}
          </span>
        </label>

        <label className={styles.field}>
          Nomor WhatsApp
          <input
            name="phone"
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            placeholder="0812 3456 7890"
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
            aria-invalid={touched && phoneError !== ""}
            aria-describedby={`${formId}-phone-help ${formId}-phone-error`}
          />
          <span id={`${formId}-phone-help`} className={styles.help}>
            Konfirmasi booking dikirim ke nomor ini.
          </span>
          <span id={`${formId}-phone-error`} className={styles.error}>
            {touched && phoneError}
          </span>
        </label>

        <p className={styles.holdNote}>
          Slot ditahan 10 menit selama kamu membayar DP. Lewat dari itu, slot dilepas.
        </p>

        {hold.isError && (
          <p className={styles.failure} role="alert">
            {hold.error.message}
          </p>
        )}

        <div className={styles.actions}>
          <button
            type="submit"
            className={styles.primary}
            disabled={hold.isPending || hold.isSuccess}
          >
            {hold.isPending || hold.isSuccess
              ? "Menahan slot…"
              : `Bayar DP ${rupiah(deposit(price))}`}
          </button>
          <button
            type="button"
            className={styles.secondary}
            onClick={() => dialog.current?.close()}
          >
            Pilih jam lain
          </button>
        </div>
      </form>
    </dialog>
  );
}
