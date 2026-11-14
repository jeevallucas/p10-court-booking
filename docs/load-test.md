# Load test: two hundred players, one court hour

`load/hold-race.js` (k6) sends 200 hold requests for the same court and hour at the same moment:
each virtual player waits for a shared start time, then posts one hold from its own client
address. The run fails unless exactly one hold is created, the other 199 are refused with
`slot_taken`, nothing else comes back, and the hour then shows as taken.

```bash
docker compose up -d db
cd api && go run ./cmd/seed          # with DATABASE_URL and DEMO_OWNER_PASSWORD from .env.example
TRUSTED_PROXY_HOPS=1 PAYMENT_SERVER_KEY=local-dev-key COOKIE_SECURE=false go run ./cmd/api
k6 run load/hold-race.js             # from the repository root, in another terminal
```

`TRUSTED_PROXY_HOPS=1` lets each virtual player bring its own address in `X-Forwarded-For`, as
players behind the hosting proxy do. Without it, the per-address rate limit would refuse most
attempts before they reached the database. CI runs the same test on every push (`load` job in
`.github/workflows/e2e.yml`) and checks the counts; it does not check latency, because shared
runners are too noisy for that.

## Setup for the numbers below

All on one laptop (AMD Ryzen 5 7535HS, 6 cores and 12 threads, 15 GB of memory, Windows 11):
k6 2.4.0, the API built from this repository, PostgreSQL 17 in Docker, the seeded demo data.
Latency is the time for one hold request as k6 saw it, so it includes the wait for one of the
API's database connections (pgx's default pool, 12 on this laptop). These are local numbers for
comparing runs, not a production benchmark.

## What the first runs found

Four runs on 2026-11-14 against the original code. In three, exactly one hold succeeded and 199
were refused, with p95 latency between 122 and 265 ms. The fourth failed the test: 1 hold, 12
refusals, and 187 requests that either returned 500 or ran past k6's 60-second timeout, with
median latency 35 s.

The API log had 57 errors `deadlock detected (SQLSTATE 40P01)`. Two transactions inserted
overlapping holds at the same moment; each then checked the exclusion constraint, found the
other's uncommitted row, and waited for it. PostgreSQL broke each cycle only after its
one-second deadlock timeout, and the other requests queued behind those stalls. The constraint
still did its job (one booking, no double booking), but players saw errors and long waits. The
200-hold test in `api/internal/booking` had passed in CI and 30 times in a row locally; the race
needs two inserts within microseconds of each other, and the HTTP run produced that.

## The fix and the runs after it

Every write that puts a booking on a court now locks the court's row first
(`select ... for no key update`, `LockCourt` in `api/internal/store/queries/booking.sql`): a
player's hold, a walk-in, a reschedule onto the court, a deposit paid at the desk, and a late
payment taking its slot back. Writers for one court wait for each other instead of racing, and
writers for different courts do not wait at all. The constraint is still what guarantees no
double booking. The expiry sweep now skips rows another transaction has locked. Tests in
`booking` and `payment` check that each of these writes waits while another transaction holds
the court.

Eight runs after the fix, the same day:

| Run | Holds created | Refused (`slot_taken`) | Other | Median | p95 | p99 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 1 | 199 | 0 | 346 ms | 517 ms | 532 ms |
| 2 | 1 | 199 | 0 | 234 ms | 397 ms | 412 ms |
| 3 | 1 | 199 | 0 | 202 ms | 360 ms | 370 ms |
| 4 | 1 | 199 | 0 | 252 ms | 413 ms | 426 ms |
| 5 | 1 | 199 | 0 | 221 ms | 373 ms | 387 ms |
| 6 | 1 | 199 | 0 | 224 ms | 388 ms | 403 ms |
| 7 | 1 | 199 | 0 | 212 ms | 375 ms | 388 ms |
| 8 | 1 | 199 | 0 | 233 ms | 391 ms | 404 ms |

The API logged no deadlocks and no 500s. Typical latency is higher than in the lucky runs
before the fix, because the 200 transactions for one court now take turns; that is the cost of
never stalling for a second on a deadlock. Two hundred players on one court in the same instant
is far beyond what a six-court venue sees; it is here to make the race happen.
