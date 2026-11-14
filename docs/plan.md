# Plan

Each milestone ends with a commit and a push. Done means the listed check passes.

## 1. Schema and the no-overlap guarantee
- Go module in `api/`, PostgreSQL in docker compose, versioned migrations (goose), sqlc.
- Tables: venue, court, price rule, booking (with a time range per court), outbox, owner, session.
- A `btree_gist` exclusion constraint on (court, time range) for bookings that still occupy the
  court: held, confirmed, and walk-in.
- Tests against a real PostgreSQL: overlapping inserts fail, touching ranges succeed, an expired
  or cancelled booking frees the slot, and 200 concurrent inserts for one slot leave exactly one.
- Done when: the concurrency test passes in CI.

## 2. Booking API, owner auth, contract
- `openapi.yaml` as the source of truth; Go handlers generated from it and requests validated
  against it. (The TypeScript client is generated in milestone 4, with the web app.)
- Availability by date and sport; hold (expires after 10 minutes); confirm; cancel; no-show;
  walk-in; reschedule.
- Owner login with Argon2id password hashes and server-side sessions; players book as guests with
  a name and WhatsApp number.
- Rate limiting on hold creation per client; structured logs with request IDs.
- Done when: handler tests cover every state change and its failure modes.

## 3. Deposit payment and notifications
- Midtrans Snap sandbox for the deposit; a local fake provider with the same signed notification
  for tests and offline development.
- Notification handler verifies the signature, is idempotent, and ignores stale or out-of-order
  notifications; a replay test sends the same notification 10 times.
- Transactional outbox and worker; Mailpit in development, logs in the demo.
- Done when: replay and expiry tests pass.

## 4. Player booking flow
- Next.js App Router, TypeScript, TanStack Query, the generated client.
- Today's free slots, date strip, sport switch, slot choice, guest details, deposit, confirmation;
  every empty, loading, and error state.
- Screenshot review at 375, 768, and 1440 px; axe; Lighthouse accessibility at least 95.

## 5. Owner schedule board
- Login, day board with six courts, booking side panel, walk-in, no-show, cancel.
- Drag to reschedule with conflict detection, a keyboard alternative, optimistic update with
  rollback on conflict.
- Screenshot review as in milestone 4.

## 6. Load test, end-to-end tests, CI
- k6: 200 concurrent attempts on one slot; exactly one success; p95 latency recorded.
- Playwright: book, pay, confirmed; owner drag-to-reschedule into a conflict.
- GitHub Actions for Go, web, contract, and end-to-end jobs.

## 7. Demo, README, review, publish
- Neon database, Render API, Vercel web app, nightly reseed workflow, demo owner login in the
  README.
- Demo recording, README, fresh-clone check, /code-review, hiring-manager audit, secret and
  forbidden-terms scan, then public.
