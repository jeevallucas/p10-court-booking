// Two hundred players try to hold the same court hour at the same moment; exactly one may win.
//
// Usage: k6 run load/hold-race.js            (API_URL defaults to http://localhost:8080)
//
// The API must run with TRUSTED_PROXY_HOPS=1. Each virtual player then sends its own address in
// X-Forwarded-For and gets its own rate-limit bucket, as real players behind the hosting proxy
// do; otherwise the per-address limit, not the database, would turn most attempts away.
import { check, sleep } from "k6";
import http from "k6/http";
import { Counter, Trend } from "k6/metrics";

const api = __ENV.API_URL || "http://localhost:8080";
const players = 200;

const created = new Counter("holds_created");
const slotTaken = new Counter("holds_refused_slot_taken");
const unexpected = new Counter("holds_unexpected");
const holdDuration = new Trend("hold_duration", true);

// A refused hold (409) is the correct answer for 199 players, not a failed request.
http.setResponseCallback(http.expectedStatuses(200, 201, 409));

export const options = {
  scenarios: {
    race: { executor: "per-vu-iterations", vus: players, iterations: 1, maxDuration: "1m" },
  },
  // The run fails unless the database let exactly one hold through and refused every other.
  thresholds: {
    holds_created: ["count==1"],
    holds_refused_slot_taken: [`count==${players - 1}`],
    holds_unexpected: ["count==0"],
  },
  summaryTrendStats: ["med", "p(95)", "p(99)", "max"],
};

// A week ahead, so neither the seed's past hours nor an earlier run's hold is in the way.
export function setup() {
  const date = new Date(Date.now() + 7 * 24 * 3600_000).toISOString().slice(0, 10);
  const response = http.get(`${api}/v1/availability?date=${date}&sport=badminton`);
  if (response.status !== 200) {
    throw new Error(`availability for ${date} failed with ${response.status}: ${response.body}`);
  }
  const slot = response.json().slots.find((candidate) => candidate.available);
  if (!slot) {
    throw new Error(`no free badminton hour on ${date}; reseed the database and run again`);
  }
  // Every player waits for the same moment, so the attempts really do overlap.
  return { date, slot, startAt: Date.now() + 3000 };
}

export default function (race) {
  sleep(Math.max(0, race.startAt - Date.now()) / 1000);
  const player = __VU;
  const response = http.post(
    `${api}/v1/holds`,
    JSON.stringify({
      courtId: race.slot.courtId,
      start: race.slot.start,
      hours: 1,
      customerName: `Pemain uji ${player}`,
      customerPhone: `0800000${String(player).padStart(5, "0")}`,
    }),
    {
      headers: {
        "Content-Type": "application/json",
        "X-Forwarded-For": `10.20.${Math.floor(player / 256)}.${player % 256}`,
      },
    },
  );
  holdDuration.add(response.timings.duration);
  if (response.status === 201) {
    created.add(1);
  } else if (response.status === 409 && response.json("code") === "slot_taken") {
    slotTaken.add(1);
  } else {
    unexpected.add(1);
    console.error(`player ${player}: ${response.status} ${response.body}`);
  }
}

export function teardown(race) {
  const response = http.get(`${api}/v1/availability?date=${race.date}&sport=badminton`);
  const slot = response
    .json()
    .slots.find((candidate) => candidate.courtId === race.slot.courtId && candidate.start === race.slot.start);
  check(slot, { "the contested hour now shows as taken": (s) => s !== undefined && !s.available });
}
