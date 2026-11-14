# Arena Kotabaru: court booking without double bookings

A booking system for a six-court badminton and futsal venue: players book and pay a deposit from
their phones, and the owner runs the day from one schedule board. The venue is fictional; the
code, tests and demo are real.

By Putu Jeevallucas Jnanamaitriya Surya Gautama ([GitHub](https://github.com/jeevallucas),
[LinkedIn](https://www.linkedin.com/in/jeevallucas-gautama/))

[![API](https://github.com/jeevallucas/p10-court-booking/actions/workflows/api.yml/badge.svg)](https://github.com/jeevallucas/p10-court-booking/actions/workflows/api.yml)
[![End-to-end and load](https://github.com/jeevallucas/p10-court-booking/actions/workflows/e2e.yml/badge.svg)](https://github.com/jeevallucas/p10-court-booking/actions/workflows/e2e.yml)
[![MIT licence](https://img.shields.io/badge/licence-MIT-blue.svg)](LICENSE)

**Live demo:** [p10-court-booking.vercel.app](https://p10-court-booking.vercel.app) for players,
[/owner](https://p10-court-booking.vercel.app/owner) for the owner's board. The login page shows
the demo account (`demo@arenakotabaru.test`, password `arena-demo-2026`). The data resets every
night at 02.00 WIB, and payments are simulated: no money moves.

| Player, on a phone | Owner, on a laptop |
| --- | --- |
| ![A player picks a day, a free court, fills in a name and WhatsApp number, pays the deposit on the simulated payment page and sees the booking confirmed](docs/screenshots/demo-player.gif) | ![The owner drags a booking to a free court, then drags another onto a busy court and the board refuses with the name of the booking in the way](docs/screenshots/demo-owner.gif) |

## The problem

The owner of a six-court venue in Yogyakarta decides many times a day which slot goes to whom,
whether a walk-in fits a gap, and what to do with a no-show. Today that runs on WhatsApp chats,
a paper book, and bank-transfer screenshots checked by hand. The hard part is concurrency: two
players can tap the same evening slot within milliseconds, payment notifications arrive late or
twice, and the owner moves bookings while players are still paying. One slot booked twice means a
group of ten arriving to a full court.

## What it does

- Players see the free courts for any of the next 14 days and book in a few taps; the slot is
  held for 10 minutes while they pay a deposit of half the price.
- The database refuses any overlapping booking on a court, so 200 simultaneous attempts on one
  slot leave exactly one booking.
- Payment notifications are signed, applied once even when replayed or delivered out of order, and
  a late payment takes its slot back if it is still free.
- The owner's board shows six courts in half-hour rows; bookings move by dragging or from a side
  panel, and a move onto another booking is refused with the name of the booking in the way.
- The owner records walk-ins, deposits paid at the desk, no-shows and cancellations.
- Confirmations go through a transactional outbox, so a booking is never confirmed without its
  message being queued.

## How it works

```mermaid
flowchart LR
    player[Player's phone] --> web
    owner[Owner's laptop] --> web
    web["Next.js web app<br/>proxies /v1 and /fakepay"] --> api
    api["Go API<br/>server generated from openapi.yaml"] --> db[("PostgreSQL<br/>exclusion constraint per court")]
    api --> fakepay["Fake payment page<br/>(Midtrans notification format)"]
    fakepay -- "signed notification" --> api
    api --> outbox["Outbox worker<br/>confirmation messages"]
```

A player's hold is one transaction: it locks the court's row, marks holds whose ten minutes are
over as expired, and inserts the new hold. A `btree_gist` exclusion constraint on (court, time
range) refuses any overlap, and the API turns that into `409 slot_taken`. On the payment page the
player pays; the API receives a notification signed like Midtrans's, records it once per
transaction and status, confirms the booking, and queues the confirmation in the same
transaction. The owner's board reads the day's bookings, moves a block as soon as it is dropped,
and puts it back if the API answers 409.

## Decisions and trade-offs

- **I chose a PostgreSQL exclusion constraint over application locks** because it holds for every
  writer, including the seed program and manual SQL, and cannot be forgotten in a new code path.
  Cost: the load test showed that two overlapping inserts can deadlock on it, so every booking
  write now also locks the court's row first, and writes for one court take turns.
- **I chose expiring holds by the database clock, on every write and read, over a timer alone**,
  so a hold whose time is over never blocks a slot and reads as expired even if the sweep is late.
  Cost: every query that reads holds must compare their expiry with the clock, not only the status.
- **I chose recording each payment notification once per (transaction, status) under a lock on
  the booking over trusting the booking's current status**, because providers retry and reorder
  deliveries. Cost: a payment that arrives after someone else took the slot cannot be undone
  automatically; the owner gets an alert to refund it.
- **I chose a Go API next to the Next.js app over one Next.js app**, so the booking rules, locks
  and outbox worker live in one service that the web app cannot bypass. Cost: two deployments, and
  the web app proxies `/v1` to keep the owner's session cookie first-party.
- **I chose to generate both the Go server and the TypeScript client from `openapi.yaml` over
  handwritten clients**; CI fails if either drifts from the spec. Cost: generated code lives in the
  repository, and API changes start in the spec.
- **I chose Vercel and Neon over Render for the demo** because Render asked for a credit card even
  for a free service. Cost: on Vercel the outbox worker and the expiry sweep run only while an
  instance is active, and rate limits are kept per instance.

The full log, including the fake payment provider and the hosting details, is in
[docs/decisions.md](docs/decisions.md).

## Results

Every number below comes from a test in this repository, run as described.

**Two hundred players, one court hour.** `load/hold-race.js` (k6) sends 200 holds for the same
court and hour at the same moment, each from its own client address. After the court lock, eight
runs on a laptop (Ryzen 5 7535HS, API and PostgreSQL 17 on the same machine) all ended with
exactly 1 hold and 199 `slot_taken` refusals, no errors, and hold latency p95 between 360 and
517 ms. Before the court lock, one of four runs hit 57 deadlocks and made some players wait up to
56 seconds. The runs and the fix are in [docs/load-test.md](docs/load-test.md); CI repeats the
race on every push and checks the counts, not the latency.

**Payments.** Replaying the same notification 10 times, in a row or concurrently, confirms the
booking once and queues one message (`api/internal/payment`).

**Tests.** 58 Go tests in 8 packages run against a real PostgreSQL database, among them 200
concurrent holds through the service layer. The web app has 41 unit and component tests, and 4
Playwright tests drive the built app against the real API: book, pay and see it confirmed; a slot
taken by someone else; an owner's drag onto another booking refused; and a move the API refuses
put back.

**Accessibility and speed.** axe found 0 violations across 25 page and width combinations.
Lighthouse 13.6 (mobile emulation, production build, localhost):

| Page | Performance | Accessibility | Best practices | SEO |
| --- | --- | --- | --- | --- |
| Player schedule | 97 | 100 | 100 | 100 |
| Booking status | 96 | 100 | 100 | 100 |
| Owner login | 100 | 100 | 100 | 100 |
| Owner board | 98 | 100 | 100 | 100 |

## Limitations

- Payments are simulated. The Midtrans sandbox account could not be opened, so a fake provider
  speaks Midtrans's signed notification format; there is no real checkout page.
- In the live demo, confirmations and the expiry sweep run only while Vercel keeps an instance
  active, and the rate limits (5 holds at once per address, then 10 a minute) reset with each
  instance. `docker compose` runs the API as a long-lived server instead.
- Dragging works with a mouse; on touch screens the side panel moves bookings.
- One venue: its opening hours (07.00 to 23.00) are copied into the board's layout.
- Logs are structured with request IDs, but there are no metrics yet. The next step is counting
  hold conflicts and notification outcomes.
- Messages go to the log in the demo and to Mailpit locally; nothing is sent to WhatsApp.
- The load-test latency comes from one laptop, not from the hosted demo.

## Run it locally

Prerequisites: Git and Docker Desktop (Docker Engine 29 with Compose v2 was used).

```bash
git clone https://github.com/jeevallucas/p10-court-booking.git
cd p10-court-booking
docker compose up --build
```

The first build takes a few minutes. When the log shows
`seeded Arena Kotabaru with 447 bookings` and the web app is listening, open:

- http://localhost:3000 to book as a player,
- http://localhost:3000/owner to use the board (the demo account is on the login page),
- http://localhost:8025 to read the confirmation messages in Mailpit.

Every `docker compose up` resets the demo data.

### Develop and test

With Go 1.27, Node.js 22 and Docker Desktop:

```bash
docker compose up -d db mailpit
(cd api && DATABASE_URL="postgres://court:court@localhost:5432/court_booking?sslmode=disable" go test ./...)
(cd web && npm ci && npm test)
```

The end-to-end tests (`npm run e2e` in `web/`) and the load test (`k6 run load/hold-race.js`)
need the API and the web app running; `.github/workflows/e2e.yml` shows the exact steps.
`.env.example` lists every setting.

## Data and licence

All data is synthetic. `api/internal/seed` creates the fictional venue, its prices, and two weeks
of bookings with invented Indonesian names; phone numbers use the +62 800 range, which no mobile
operator assigns to people. The Archivo typeface is under the SIL Open Font License (served by
Google Fonts), and the GitHub and LinkedIn icons are paths from Bootstrap Icons (MIT). The code is
under the [MIT licence](LICENSE).

## Design notes

The owner glances at the board between customers at a front desk, for hours, in daylight;
players book on a phone in one hand. [docs/design-brief.md](docs/design-brief.md) has the full
brief and [docs/design-review.md](docs/design-review.md) the screenshot reviews.

- **Type:** one family, Archivo, with its width axis as the expressive device: times are set
  wide and heavy like a hall scoreboard, and everything else stays at normal width.
- **Colour:** colour means booking state only (court green confirmed, striped amber waiting for a
  deposit, slate walk-in, grey no-show), and every state is also written in the block.
- **Layout:** the board follows one column per court with a side panel, as in the Fresha
  calendar; the player's flow takes the date strip and two-tap choice from Playtomic.
- **Rejected:** rounded pastel cards with a colour per court, and a dark board with a neon
  accent, which would tire an owner reading it all day.
- **Accessibility:** court buttons are named by their visible text plus the hour; the board has
  a keyboard and touch alternative to dragging; focus is always visible; nothing relies on colour
  alone.

![The owner's board at 1366 pixels wide, with courts as columns and half-hour rows](docs/screenshots/owner-board-1366.png)
