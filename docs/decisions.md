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

## 2026-11-14 Use a local fake payment provider with Midtrans's notification contract

- Context: the plan used the Midtrans sandbox (Snap) for deposits. The sandbox account could not
  be used: registration reported the email and phone as taken, and after a password reset the
  merchant login refused the account's role, which only Midtrans support can change.
- Options: (a) wait for Midtrans support; (b) a local fake provider that follows Midtrans's
  documented notification contract: the same JSON fields, statuses, and the signature
  `SHA512(order_id + status_code + gross_amount + server_key)`; (c) another gateway's sandbox.
- Decision: (b), as the plan's fallback. The parts the project exists to show are the
  notification handler's signature check, idempotency under replay, and out-of-order handling,
  and they behave the same against the fake. The payment code sits behind one interface, so a
  real Midtrans client can replace the fake when an account is available.
- Consequences: the demo's "pay deposit" step opens the fake provider's page instead of the Snap
  pop-up, and the README says so. No real or sandbox money flows anywhere.
- Revisit if: a working Midtrans sandbox account becomes available.

## 2026-11-14 Generate the Go server from openapi.yaml with oapi-codegen

- Context: the contract is the source of truth for both sides; hand-written handlers drift.
- Options: (a) hand-written handlers plus a contract test; (b) oapi-codegen's strict server for
  `net/http`, plus its request-validation middleware (kin-openapi) that rejects requests the
  spec does not allow.
- Decision: (b). Handlers implement a generated interface, so a response the spec does not
  declare does not compile, and malformed requests never reach the booking rules. Both are
  widely used Go libraries; oapi-codegen is pinned as a `go tool` in go.mod.
- Consequences: which operations need an owner session is read from the spec's `security`
  blocks at start-up. This was added after a test showed the first version's hard-coded list used
  the wrong operation names and protected nothing; `TestEveryOwnerEndpointNeedsASession` now
  calls every owner endpoint without a cookie.
- Revisit if: the spec needs OpenAPI 3.1 features oapi-codegen does not support.

## 2026-11-14 Keep the owner's session cookie first-party behind the web app's proxy

- Context: the web app (Vercel) and the API (Render) are on different sites. A cookie set by
  the API would be third-party to the web app, which browsers increasingly block.
- Options: (a) `SameSite=None` cookies across sites plus CORS; (b) bearer tokens in browser
  storage; (c) the web app proxies `/v1` to the API, so the cookie belongs to the web app's site.
- Decision: (c). The cookie is `HttpOnly`, `Secure`, and `SameSite=Lax`, which also blocks
  cross-site form posts, and no token is readable by page scripts.
- Consequences: every API call from the browser makes one extra hop through Vercel.
- Revisit if: the API needs to serve other clients that cannot use cookies.

## 2026-11-14 Rate-limit holds and logins in memory, keyed by client address

- Context: hold creation and owner login need protection from scripted abuse; one API instance
  runs the demo.
- Options: (a) a shared store such as Redis; (b) an in-memory token bucket per client address
  (`golang.org/x/time/rate`).
- Decision: (b): 10 holds and 5 logins per minute per address, with small bursts. The client
  address is the `X-Forwarded-For` entry added by the last trusted proxy (`TRUSTED_PROXY_HOPS`),
  so a client cannot pick its own address by sending that header.
- Consequences: limits reset when the API restarts, and several instances would each allow
  the full rate.
- Revisit if: the API runs on more than one instance.

## 2026-11-14 Apply payment notifications once, under a lock on the booking

- Context: payment providers retry notifications until they get a 200, may send several for one
  transaction (pending, then settlement), and do not promise the order they arrive in.
- Options: (a) apply every notification as it comes; (b) remember processed notifications by
  (transaction ID, status) and apply each once, serialising notifications for one booking with
  `SELECT ... FOR UPDATE`; (c) a queue between the endpoint and the rules.
- Decision: (b). The unique key turns a replay into a rolled-back transaction that changes
  nothing, and the row lock means two copies arriving together are applied one after the other.
  Settlement is final: a cancel or expire that arrives afterwards is recorded and ignored.
- Consequences: replaying one notification 10 times, one after another or concurrently,
  confirms once and queues one message (tests in `internal/payment`).
- Revisit if: the provider starts sending refunds that should change bookings automatically,
  which is a non-goal today.

## 2026-11-14 Take back an expired hold when its payment arrives late, or ask for a refund

- Context: a player can finish paying seconds after the ten-minute hold ran out. By then the
  slot may already belong to someone else.
- Options: (a) always refuse late payments; (b) confirm if the slot is still free, otherwise
  record "refund needed" and alert the owner.
- Decision: (b). Confirming inside a savepoint lets the exclusion constraint decide: if it
  refuses, the transaction continues, the booking stays expired, and the outbox tells the owner
  to return the deposit. Refunds stay manual, as the brief's non-goals say.
- Revisit if: refunds become automatic.

## 2026-11-14 Serve the fake payment page from the API process

- Context: the fake provider needs the server key to sign notifications, so it cannot run in
  the browser, and the free demo host gives one web service.
- Options: (a) a separate fake-provider service; (b) a small `/fakepay/{reference}` handler in
  the API binary that posts its signed notification over HTTP to the API's own notification
  endpoint, exactly as the real provider would.
- Decision: (b). The notification still travels over HTTP and through the same validation,
  signature check, and rules as a real one.
- Consequences: the page is reached through the web app's proxy like the API. It is mounted
  only for the fake provider; a real provider's checkout would replace the payment link.

## 2026-11-14 Seed a synthetic schedule that resets as a whole

- Context: the demo needs a believable day on the board and free slots for visitors to book, and
  must not drift into an empty or vandalised state.
- Options: (a) a fixed SQL dump; (b) a seed program that rebuilds two weeks around the current
  time from invented occupancy rates (busy evenings and weekend mornings, quiet weekday middays),
  inside one transaction.
- Decision: (b). Past hours get finished bookings, some no-shows and desk walk-ins; upcoming
  hours get paid bookings. The random seed is the date, so a given day's schedule can be
  reproduced. Names are invented and phone numbers use the +62 800 range, which no mobile
  operator assigns to people.
- Consequences: reseeding truncates bookings, payment notifications, and outbox messages; the
  venue, courts, prices, and owner account are kept.
- Revisit if: the demo needs to keep visitor bookings across nights.

## 2026-11-14 Poll the booking status every 3 seconds while it is held

- Context: after paying, the player lands on the status page before or just after the payment
  notification arrives, and should see the confirmation without reloading.
- Options: (a) server-sent events or a WebSocket pushed by the API; (b) poll
  `GET /v1/bookings/{code}` every 3 seconds, only while the status is `held`.
- Decision: (b). A hold lasts at most 10 minutes, so one page polls at most 200 times, and
  polling stops as soon as the booking is confirmed, expired or cancelled. It needs no
  long-lived connections through the Vercel proxy or on the Render free instance.
- Revisit if: many players wait on held bookings at once, or the owner board needs live updates.

## 2026-11-14 Test web components against a fake fetch, not a mock server library

- Context: the booking sheet and status page need tests for what the player sees when the API
  says the slot is taken, the code is unknown, or the server cannot be reached.
- Options: (a) Mock Service Worker; (b) a 30-line fake installed as `fetch` before the client
  module loads, with each test saying how the API answers.
- Decision: (b). The tests assert the rendered text, not the calls, and one more dependency
  would buy only request matching the tests do not need. The full path through the real API is
  covered by the end-to-end tests in milestone 6.
- Revisit if: tests start needing request matching or many endpoints per test.

## 2026-11-14 Render date-dependent pages per request

- Context: the schedule and the owner board start at today in WIB. Next.js prerendered the
  schedule at build time, so a deploy would keep showing its build day and the browser would
  fail hydration when its own date differed.
- Options: (a) render the date parts only in the browser, with placeholders on the server;
  (b) render those two pages per request (`connection()` in the page).
- Decision: (b). Server and browser compute the same day, nothing shifts after hydration, and
  the server work is a few milliseconds of HTML. The status page was already per request.
- Revisit if: hosting costs per request start to matter.

## 2026-11-14 Drag bookings with pointer events, and move them from the panel on touch

- Context: the owner moves a booking by dragging it on a uniform grid of half-hour rows, and
  needs a keyboard way to do the same.
- Options: (a) a drag-and-drop library such as dnd-kit; (b) pointer events and the grid's own
  geometry, about 60 lines, with the side panel's court and start-time fields as the keyboard
  and touch alternative.
- Decision: (b). With equal rows, the target cell is one division, and the library would still
  need custom snapping and collision rules. Touch keeps scrolling the board instead of starting
  drags; on a tablet the panel moves bookings.
- Consequences: no drag on touch screens. The drag state lives in a ref, because a quick drop
  can arrive before React renders the last move.
- Revisit if: owners ask for dragging on tablets.

## 2026-11-14 Show the demo owner account on the login page

- Context: reviewers should reach the owner board without asking for credentials.
- Options: (a) only in the README; (b) on the login page, with a button that fills it in.
- Decision: (b). The account owns only the fictional venue's data, the nightly reseed restores
  it, and logins are rate-limited. The seed reads the same password from DEMO_OWNER_PASSWORD.
- Consequences: anyone can change the demo board until the next reseed.

## 2026-11-14 Label paid bookings "Terkonfirmasi", not "Lunas"

- Context: the design brief named the confirmed state "Lunas" (paid in full), but a confirmed
  booking has usually paid only the deposit and owes the rest at the desk.
- Decision: the board and panel say "Terkonfirmasi" and the panel shows what is still due at
  the desk, so the owner does not wave through a player who still owes money.
