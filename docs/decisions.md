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
