"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { api, type Sport } from "@/api/client";
import { SlowServerNote } from "@/components/SlowServerNote";
import { dayParts, longDate, rupiah, timeLabel, upcomingDays } from "@/lib/format";
import { problemMessage } from "@/lib/problems";
import { hourRows, type FreeCourt } from "@/lib/slots";

import { BookingDialog } from "./BookingDialog";
import styles from "./BookingPage.module.css";

const sportLabels: Record<Sport, string> = { badminton: "Badminton", futsal: "Futsal" };

export function BookingPage() {
  const [now] = useState(() => new Date());
  const days = useMemo(() => upcomingDays(now, 14), [now]);
  const [day, setDay] = useState(days[0]);
  const [sport, setSport] = useState<Sport>("badminton");
  const [chosen, setChosen] = useState<FreeCourt | null>(null);

  const availability = useQuery({
    queryKey: ["availability", day, sport],
    queryFn: async () => {
      const { data, error } = await api.GET("/v1/availability", {
        params: { query: { date: day, sport } },
      });
      if (!data) {
        throw new Error(problemMessage(error));
      }
      return data;
    },
  });
  const rows = availability.data ? hourRows(availability.data, new Date()) : [];
  const openRows = rows.filter((row) => !row.past);
  const freeCount = openRows.reduce((sum, row) => sum + row.free.length, 0);
  const dayTitle = longDate(`${day}T12:00:00+07:00`);
  const otherSport: Sport = sport === "badminton" ? "futsal" : "badminton";

  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <h1>Arena Kotabaru</h1>
        <p>
          Badminton dan futsal di Yogyakarta, buka{" "}
          <span className={styles.nowrap}>07.00–23.00 WIB</span>
        </p>
      </header>

      <section aria-label="Pilih tanggal" className={styles.days}>
        {days.map((key, index) => {
          const parts = dayParts(key);
          return (
            <button
              key={key}
              type="button"
              className={styles.day}
              aria-pressed={key === day}
              onClick={() => setDay(key)}
            >
              <span>{index === 0 ? "Hari ini" : parts.weekday}</span>
              <strong className="time">{parts.day}</strong>
              <span>{parts.month}</span>
            </button>
          );
        })}
      </section>

      <div className={styles.sports} role="group" aria-label="Pilih olahraga">
        {(Object.keys(sportLabels) as Sport[]).map((key) => (
          <button
            key={key}
            type="button"
            aria-pressed={key === sport}
            onClick={() => setSport(key)}
          >
            {sportLabels[key]}
          </button>
        ))}
      </div>

      <section aria-labelledby="slots-heading" aria-busy={availability.isPending}>
        <h2 id="slots-heading" className={styles.sectionTitle}>
          {sportLabels[sport]}, {dayTitle}
        </h2>

        {availability.isPending && (
          <>
            <SlowServerNote />
            <SlotSkeleton />
          </>
        )}

        {availability.isError && (
          <div className={styles.notice} role="alert">
            <p>Jadwal gagal dimuat. {availability.error.message}</p>
            <button type="button" onClick={() => availability.refetch()}>
              Muat ulang jadwal
            </button>
          </div>
        )}

        {availability.data && openRows.length === 0 && (
          <div className={styles.notice}>
            <p>Jam buka hari ini sudah lewat. Pilih tanggal lain untuk memesan.</p>
          </div>
        )}

        {availability.data && openRows.length > 0 && freeCount === 0 && (
          <div className={styles.notice}>
            <p>
              Semua lapangan {sportLabels[sport].toLowerCase()} penuh pada tanggal ini. Coba tanggal
              lain atau lihat jadwal {sportLabels[otherSport].toLowerCase()}.
            </p>
            <button type="button" onClick={() => setSport(otherSport)}>
              Lihat {sportLabels[otherSport].toLowerCase()}
            </button>
          </div>
        )}

        {availability.data && freeCount > 0 && (
          <ol className={styles.hours}>
            {openRows.map((row) => (
              <li key={row.start} className={styles.hour}>
                <span className={`time ${styles.hourLabel}`}>{timeLabel(row.start)}</span>
                {row.free.length === 0 ? (
                  <span className={styles.full}>Penuh</span>
                ) : (
                  <ul className={styles.courts}>
                    {row.free.map((free) => (
                      <li key={free.court.id}>
                        <button
                          type="button"
                          className={styles.court}
                          onClick={() => setChosen(free)}
                        >
                          {/* The name is the visible text, so voice control users can say
                              what they see; the hidden part gives the hour to screen readers. */}
                          <span>{free.court.name}</span>{" "}
                          <span className={styles.price}>{rupiah(free.slot.priceIdr)}</span>
                          <span className="visually-hidden">
                            {" "}
                            per jam, pukul {timeLabel(row.start)}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </li>
            ))}
          </ol>
        )}
      </section>

      {chosen && availability.data && (
        <BookingDialog
          chosen={chosen}
          availability={availability.data}
          onClose={() => setChosen(null)}
          onSlotTaken={() => availability.refetch()}
        />
      )}
    </main>
  );
}

function SlotSkeleton() {
  return (
    <ol className={styles.hours} aria-hidden="true">
      {Array.from({ length: 6 }, (_, index) => (
        <li key={index} className={`${styles.hour} ${styles.skeleton}`}>
          <span className={styles.skeletonTime} />
          <span className={styles.courts}>
            <span className={styles.skeletonCourt} />
            <span className={styles.skeletonCourt} />
          </span>
        </li>
      ))}
    </ol>
  );
}
