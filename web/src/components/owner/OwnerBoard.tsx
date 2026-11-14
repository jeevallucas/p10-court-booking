"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent,
  type PointerEvent,
} from "react";

import type { Court, OwnerBooking, OwnerDay } from "@/api/client";
import { SlowServerNote } from "@/components/SlowServerNote";
import {
  canMove,
  moveTo,
  placementProblem,
  rowCount,
  rowLabel,
  rowOf,
  rowsSpanned,
  statusLabels,
  venueMinute,
  opensAt,
  slotMinutes,
  type Placement,
} from "@/lib/board";
import { dayKey, longDate, timeLabel } from "@/lib/format";

import { BookingPanel } from "./BookingPanel";
import styles from "./OwnerBoard.module.css";
import {
  changeBookingStatus,
  fetchOwnerDay,
  logOut,
  OwnerApiError,
  rescheduleBooking,
  type OwnerStatus,
} from "./ownerApi";
import { WalkInForm } from "./WalkInForm";

// One half-hour row in pixels; the grid, the blocks and the pointer maths share it.
const rowPx = 32;
// A press has to travel this far before it becomes a drag, so a click still selects.
const dragThresholdPx = 6;

type Selection =
  { kind: "booking"; id: string } | { kind: "walk-in"; courtId?: string; row?: number } | null;
type Notice = { tone: "done" | "failed"; text: string } | null;

interface Drag {
  booking: OwnerBooking;
  pointerId: number;
  originX: number;
  originY: number;
  grabRow: number;
  active: boolean;
  target?: DragTarget;
}

interface DragTarget {
  courtId: string;
  row: number;
  placement: Placement | null;
  problem?: string;
}

const statusDone: Record<OwnerStatus, (name: string) => string> = {
  confirmed: (name) => `DP ${name} ditandai dibayar di kasir.`,
  cancelled: (name) => `Booking ${name} dibatalkan; slotnya kosong lagi.`,
  no_show: (name) => `${name} ditandai tidak datang.`,
};

function shiftDay(day: string, days: number): string {
  const [year, month, date] = day.split("-").map(Number);
  return new Date(Date.UTC(year, month - 1, date + days)).toISOString().slice(0, 10);
}

export function OwnerBoard() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const now = useNow();
  const today = dayKey(now);
  const [day, setDay] = useState(today);
  const [selection, setSelection] = useState<Selection>(null);
  const [notice, setNotice] = useState<Notice>(null);
  // The drag lives in a ref: pointer events can outrun rendering, and a quick drop must see the
  // last position. State only drives what is drawn.
  const dragging = useRef<Drag | null>(null);
  // The browser sends a click after every drag; it must not count as choosing a booking.
  const dragJustEnded = useRef(false);
  const [dragView, setDragView] = useState<{ booking: OwnerBooking; target: DragTarget } | null>(
    null,
  );
  const scrollArea = useRef<HTMLDivElement>(null);
  const scrolledFor = useRef<string | null>(null);
  const queryKey = ["owner-day", day];

  const board = useQuery({
    queryKey,
    queryFn: () => fetchOwnerDay(day),
    // Players keep booking while the owner watches; pick their holds up without a reload.
    refetchInterval: 30_000,
    retry: (failures, error) =>
      !(error instanceof OwnerApiError && error.sessionEnded) && failures < 1,
  });

  const sessionEnded = board.error instanceof OwnerApiError && board.error.sessionEnded;
  useEffect(() => {
    if (sessionEnded) {
      router.replace("/owner/login");
    }
  }, [sessionEnded, router]);

  // Open today's board at the current hour, once per day shown.
  useEffect(() => {
    if (!board.data || scrolledFor.current === day || !scrollArea.current) {
      return;
    }
    scrolledFor.current = day;
    if (day === today) {
      const row = (venueMinute(new Date().toISOString()) - opensAt) / slotMinutes;
      scrollArea.current.scrollTop = Math.max(0, (Math.floor(row) - 2) * rowPx);
    }
  }, [board.data, day, today]);

  function failed(error: Error) {
    if (error instanceof OwnerApiError && error.sessionEnded) {
      router.replace("/owner/login");
    }
    setNotice({ tone: "failed", text: error.message });
  }

  const move = useMutation({
    mutationFn: ({ booking, placement }: { booking: OwnerBooking; placement: Placement }) =>
      rescheduleBooking(booking.id, placement),
    // Move the block at once; put it back if the API refuses.
    onMutate: async ({ booking, placement }) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<OwnerDay>(queryKey);
      const courtName = previous?.courts.find((court) => court.id === placement.courtId)?.name;
      queryClient.setQueryData<OwnerDay>(
        queryKey,
        (current) =>
          current && {
            ...current,
            bookings: current.bookings.map((other) =>
              other.id === booking.id
                ? { ...other, ...placement, courtName: courtName ?? other.courtName }
                : other,
            ),
          },
      );
      return { previous };
    },
    onError: (error, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKey, context.previous);
      }
      failed(error);
    },
    onSuccess: (moved) => {
      setSelection({ kind: "booking", id: moved.id });
      setNotice({
        tone: "done",
        text: `${moved.customerName} dipindah ke ${moved.courtName}, ${timeLabel(moved.start)}–${timeLabel(moved.end)}.`,
      });
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  });

  const status = useMutation({
    mutationFn: ({ booking, to }: { booking: OwnerBooking; to: OwnerStatus }) =>
      changeBookingStatus(booking.id, to),
    onSuccess: (changed, { to }) => {
      setNotice({ tone: "done", text: statusDone[to](changed.customerName) });
      if (to === "cancelled") {
        setSelection(null);
      }
    },
    onError: failed,
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  });

  const courts = board.data?.courts ?? [];
  const bookings = board.data?.bookings ?? [];
  const selected =
    selection?.kind === "booking" ? bookings.find((booking) => booking.id === selection.id) : null;

  // A new choice by the owner replaces the last message, which was about something else.
  function choose(next: Selection) {
    setSelection(next);
    setNotice(null);
  }

  function changeDay(next: string) {
    setDay(next);
    setSelection(null);
    setNotice(null);
  }

  function targetAt(x: number, y: number, booking: OwnerBooking, grabRow: number): DragTarget {
    const column = document.elementFromPoint(x, y)?.closest<HTMLElement>("[data-court-id]");
    if (!column?.dataset.courtId) {
      return { courtId: booking.courtId, row: rowOf(booking.start), placement: null };
    }
    const court = courts.find((candidate) => candidate.id === column.dataset.courtId);
    const row = Math.floor((y - column.getBoundingClientRect().top) / rowPx) - grabRow;
    const placement = moveTo(booking, day, column.dataset.courtId, row);
    const problem =
      court && court.sport !== booking.sport
        ? `Booking ${booking.sport} hanya bisa dipindah ke lapangan ${booking.sport}.`
        : placementProblem(bookings, placement, now, booking.id);
    return { courtId: column.dataset.courtId, row, placement, problem };
  }

  function startDrag(event: PointerEvent<HTMLButtonElement>, booking: OwnerBooking) {
    dragJustEnded.current = false;
    // Touch keeps scrolling the board; on a tablet the panel's move form does the moving.
    if (event.button !== 0 || event.pointerType === "touch" || !canMove(booking, now)) {
      return;
    }
    const top = event.currentTarget.getBoundingClientRect().top;
    event.currentTarget.setPointerCapture(event.pointerId);
    dragging.current = {
      booking,
      pointerId: event.pointerId,
      originX: event.clientX,
      originY: event.clientY,
      grabRow: Math.floor((event.clientY - top) / rowPx),
      active: false,
    };
  }

  function continueDrag(event: PointerEvent<HTMLButtonElement>) {
    const current = dragging.current;
    if (!current || event.pointerId !== current.pointerId) {
      return;
    }
    const travelled = Math.hypot(event.clientX - current.originX, event.clientY - current.originY);
    if (!current.active && travelled < dragThresholdPx) {
      return;
    }
    current.active = true;
    current.target = targetAt(event.clientX, event.clientY, current.booking, current.grabRow);
    setDragView({ booking: current.booking, target: current.target });
  }

  function endDrag(event: PointerEvent<HTMLButtonElement>) {
    const finished = dragging.current;
    if (!finished || event.pointerId !== finished.pointerId) {
      return;
    }
    dragging.current = null;
    setDragView(null);
    const landing = finished.target;
    dragJustEnded.current = finished.active;
    if (!finished.active || !landing) {
      return;
    }
    const unchanged =
      landing.courtId === finished.booking.courtId && landing.row === rowOf(finished.booking.start);
    if (unchanged) {
      return;
    }
    if (landing.problem || !landing.placement) {
      setNotice({
        tone: "failed",
        text: `${finished.booking.customerName} tidak dipindah. ${landing.problem ?? ""}`.trim(),
      });
      return;
    }
    move.mutate({ booking: finished.booking, placement: landing.placement });
  }

  function cancelDrag() {
    dragging.current = null;
    setDragView(null);
  }

  function addWalkInAt(event: MouseEvent<HTMLDivElement>, court: Court) {
    if (event.target !== event.currentTarget) {
      return;
    }
    const row = Math.floor(
      (event.clientY - event.currentTarget.getBoundingClientRect().top) / rowPx,
    );
    choose({ kind: "walk-in", courtId: court.id, row });
  }

  const nowRow = day === today ? (venueMinute(now.toISOString()) - opensAt) / slotMinutes : null;
  const panelOpen = selection !== null;

  return (
    <main className={styles.page} data-wide-page>
      <header className={styles.toolbar}>
        <div className={styles.dayNav}>
          <div className={styles.stepper}>
            <button
              type="button"
              className={styles.step}
              aria-label="Hari sebelumnya"
              onClick={() => changeDay(shiftDay(day, -1))}
            >
              ‹
            </button>
            <h1>{longDate(`${day}T12:00:00+07:00`)}</h1>
            <button
              type="button"
              className={styles.step}
              aria-label="Hari berikutnya"
              onClick={() => changeDay(shiftDay(day, 1))}
            >
              ›
            </button>
          </div>
          {day !== today && (
            <button type="button" className={styles.quiet} onClick={() => changeDay(today)}>
              Hari ini
            </button>
          )}
        </div>
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.primary}
            onClick={() => choose({ kind: "walk-in" })}
          >
            Tambah walk-in
          </button>
          <button
            type="button"
            className={styles.quiet}
            onClick={async () => {
              await logOut();
              queryClient.removeQueries({ queryKey: ["owner-day"] });
              router.push("/owner/login");
            }}
          >
            Keluar
          </button>
        </div>
      </header>

      <div className={styles.noticeArea} aria-live="polite">
        {notice && (
          <p className={styles.notice} data-tone={notice.tone}>
            {notice.text}
          </p>
        )}
      </div>

      <div className={styles.layout} data-panel-open={panelOpen}>
        <section aria-label="Jadwal per lapangan" className={styles.boardArea}>
          {board.isPending && (
            <>
              <SlowServerNote />
              <BoardSkeleton />
            </>
          )}
          {board.isError && !sessionEnded && (
            <div className={styles.failure} role="alert">
              <p>Jadwal gagal dimuat. {board.error.message}</p>
              <button type="button" className={styles.quiet} onClick={() => board.refetch()}>
                Muat ulang jadwal
              </button>
            </div>
          )}
          {board.data && (
            <>
              {bookings.length === 0 && (
                <p className={styles.empty}>
                  Belum ada booking pada tanggal ini. Klik petak kosong atau &quot;Tambah
                  walk-in&quot; untuk mencatat pelanggan di meja depan.
                </p>
              )}
              <div className={styles.scroll} ref={scrollArea}>
                <div
                  className={styles.grid}
                  style={
                    {
                      "--row": `${rowPx}px`,
                      "--rows": rowCount,
                      "--courts": courts.length,
                    } as CSSProperties
                  }
                >
                  <div className={styles.corner} />
                  {courts.map((court, index) => (
                    <div
                      key={court.id}
                      className={styles.courtHead}
                      data-sport-start={index > 0 && courts[index - 1].sport !== court.sport}
                    >
                      {court.name}
                    </div>
                  ))}

                  <div className={styles.times} aria-hidden="true">
                    {Array.from({ length: rowCount }, (_, row) => (
                      <span
                        key={row}
                        className={row % 2 === 0 ? `time ${styles.hour}` : styles.halfHour}
                        style={{ top: row * rowPx }}
                      >
                        {rowLabel(row)}
                      </span>
                    ))}
                  </div>

                  {courts.map((court, index) => (
                    <div
                      key={court.id}
                      className={styles.column}
                      data-court-id={court.id}
                      data-sport-start={index > 0 && courts[index - 1].sport !== court.sport}
                      onClick={(event) => addWalkInAt(event, court)}
                    >
                      {nowRow !== null && nowRow >= 0 && nowRow < rowCount && (
                        <div className={styles.now} style={{ top: nowRow * rowPx }} />
                      )}
                      {bookings
                        .filter((booking) => booking.courtId === court.id)
                        .map((booking) => (
                          <button
                            key={booking.id}
                            type="button"
                            className={styles.block}
                            data-status={booking.status}
                            data-movable={canMove(booking, now)}
                            data-dragging={dragView?.booking.id === booking.id}
                            aria-current={selected?.id === booking.id ? "true" : undefined}
                            style={{
                              top: rowOf(booking.start) * rowPx + 1,
                              height: rowsSpanned(booking) * rowPx - 2,
                            }}
                            onClick={() => {
                              if (dragJustEnded.current) {
                                dragJustEnded.current = false;
                                return;
                              }
                              choose({ kind: "booking", id: booking.id });
                            }}
                            onPointerDown={(event) => startDrag(event, booking)}
                            onPointerMove={continueDrag}
                            onPointerUp={endDrag}
                            onPointerCancel={cancelDrag}
                            onKeyDown={(event) => event.key === "Escape" && cancelDrag()}
                          >
                            <span className={styles.name}>{booking.customerName}</span>
                            <span className={styles.meta}>
                              {timeLabel(booking.start)}–{timeLabel(booking.end)}
                            </span>
                            <span className={styles.meta}>{statusLabels[booking.status]}</span>
                            <span className="visually-hidden">, {booking.courtName}</span>
                          </button>
                        ))}
                      {dragView?.target.courtId === court.id && (
                        <div
                          className={styles.ghost}
                          data-problem={Boolean(dragView.target.problem)}
                          style={{
                            top: dragView.target.row * rowPx,
                            height: rowsSpanned(dragView.booking) * rowPx,
                          }}
                        >
                          {dragView.target.problem ??
                            (dragView.target.placement &&
                              timeLabel(dragView.target.placement.start))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            </>
          )}
        </section>

        <aside className={styles.panel} aria-label="Detail">
          {selection === null && (
            <p className={styles.panelHint}>
              Pilih booking untuk melihat detail dan mengubahnya, atau klik petak kosong untuk
              menambah walk-in. Geser booking dengan mouse untuk memindahkannya.
            </p>
          )}
          {selected && (
            <BookingPanel
              key={selected.id}
              booking={selected}
              courts={courts}
              bookings={bookings}
              day={day}
              now={now}
              busy={move.isPending || status.isPending}
              onMove={(placement) => move.mutate({ booking: selected, placement })}
              onStatus={(to) => status.mutate({ booking: selected, to })}
              onClose={() => setSelection(null)}
            />
          )}
          {selection?.kind === "walk-in" && board.data && (
            <WalkInForm
              key={`${selection.courtId}-${selection.row}`}
              courts={courts}
              bookings={bookings}
              day={day}
              now={now}
              initialCourtId={selection.courtId}
              initialRow={selection.row}
              onCreated={(created) => {
                queryClient.invalidateQueries({ queryKey });
                setSelection({ kind: "booking", id: created.id });
                setNotice({
                  tone: "done",
                  text: `Walk-in ${created.customerName} disimpan di ${created.courtName}, ${timeLabel(created.start)}–${timeLabel(created.end)}.`,
                });
              }}
              onSessionEnded={() => router.replace("/owner/login")}
              onClose={() => setSelection(null)}
            />
          )}
        </aside>
      </div>
    </main>
  );
}

function BoardSkeleton() {
  return (
    <div className={styles.skeleton} aria-hidden="true">
      {Array.from({ length: 7 }, (_, index) => (
        <span key={index} />
      ))}
    </div>
  );
}

/** The current time, refreshed every half minute: enough for "has it started" checks. */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 30_000);
    return () => clearInterval(timer);
  }, []);
  return now;
}
