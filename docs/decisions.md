# Decisions

Newest at the bottom. Each entry: context, options, decision, consequences, and what would make me
revisit it.

## 2026-11-14 One repository with the API, the web app, and the API contract

- Context: a Go API and a Next.js app share one contract, and the end-to-end tests need both.
- Options: (a) two repositories; (b) one repository with `api/`, `web/`, and `openapi.yaml` at the
  root.
- Decision: (b). A change to the contract, the server, and the client lands in one commit, and CI
  can check that the generated client matches the spec.
- Consequences: CI runs Go and Node jobs; each folder keeps its own tooling.
- Revisit if: the web app ever talks to more than this one API.

## 2026-11-14 Run the notification worker inside the API process on the demo host

- Context: the plan was an API and a separate outbox worker on Render. Render's free tier covers
  web services only; background workers and cron jobs are paid.
- Options: (a) pay for a worker; (b) run the outbox loop as a goroutine in the API process, with a
  flag to run it separately; (c) drop the outbox.
- Decision: (b). The worker claims outbox rows with `FOR UPDATE SKIP LOCKED`, so one or several
  copies can run without sending a message twice. Locally, docker compose can still run it as its
  own process.
- Consequences: while the free API sleeps, no notifications go out; they are sent when it wakes.
  The demo only logs notifications, so nothing is lost.
- Revisit if: the demo moves to a paid plan.

## 2026-11-14 Reseed the demo nightly from GitHub Actions

- Context: the demo must never look empty or vandalised, and Render's cron jobs are not free.
- Options: (a) a Render cron job; (b) a scheduled GitHub Actions workflow that runs the seed
  program against the Neon database; (c) reseed on API start.
- Decision: (b). Scheduled workflows are free for public repositories, and the seed program is the
  same one developers run locally.
- Consequences: the Neon connection string is stored as a GitHub Actions secret.
- Revisit if: GitHub's scheduled runs prove too unreliable to keep the demo fresh.

## 2026-11-14 Indonesian interface, English code and documents

- Context: the users are a venue owner and players in Yogyakarta; the repository is read by
  reviewers who may not read Indonesian.
- Options: (a) English interface; (b) Indonesian interface; (c) both, with a language switch.
- Decision: (b). Labels, errors, times (19.00, WIB), and amounts (Rp80.000) follow Indonesian
  conventions. The README explains the screens in English.
- Consequences: end-to-end tests select elements by their Indonesian accessible names.
- Revisit if: the demo needs an English audience more than a realistic one.

## 2026-11-14 Prevent double booking with an exclusion constraint, not application locks

- Context: two players may try to hold the same court and time at the same moment, and the owner
  may create a walk-in while a player pays.
- Options: (a) `SELECT ... FOR UPDATE` on existing rows, then insert; (b) an advisory lock per
  court; (c) a PostgreSQL exclusion constraint on (court, time range) using `btree_gist`, with
  the application translating the violation into "slot taken".
- Decision: (c). Option (a) cannot lock rows that do not exist yet, so two inserts into an empty
  slot both succeed unless the isolation level is raised; (b) works only for writers that remember
  to take the lock. The constraint holds for every writer, including the seed program and manual
  SQL, and costs one GiST index.
- Consequences: the constraint cannot read the clock, so a hold whose ten minutes are over still
  counts until its row is marked expired. Placing a hold therefore expires stale holds on that
  court and period in the same transaction; a periodic sweep will expire the rest. The test
  `TestTwoHundredConcurrentHoldsForOneSlotLeaveExactlyOne` checks the guarantee with 200
  goroutines.
- Revisit if: bookings ever span venues or need partial-court sharing, which a single range per
  court cannot express.

## 2026-11-14 Test against a real PostgreSQL database per test

- Context: the guarantee lives in a constraint, so a mocked store cannot test it.
- Options: (a) testcontainers; (b) a `DATABASE_URL` pointing at a running PostgreSQL (docker
  compose locally, a service container in CI), with each test creating and dropping its own
  database.
- Decision: (b). It adds no dependency, runs the same way locally and in CI, and a fresh database
  per test keeps tests independent; each costs about 0.4 s.
- Consequences: tests fail with a message naming the fix when `DATABASE_URL` is unset.
- Revisit if: the suite grows past a minute.

## 2026-11-14 Generate typed queries with sqlc run from its Docker image

- Context: sqlc's PostgreSQL parser needs cgo, which this Windows machine does not have.
- Options: (a) hand-written pgx queries; (b) sqlc installed with `go install`; (c) sqlc from its
  official Docker image, version pinned.
- Decision: (c). The generated code is committed, and CI runs `sqlc diff` with the same image to
  catch generated code that is out of date.
