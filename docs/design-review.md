# Design review: player flow

Screens: today's schedule, another day's schedule, booking sheet (empty and with errors), all
courts full, schedule failed to load, loading, booking status (held, confirmed, expired) and
booking not found. Each at 375, 768 and 1440 px, captured with `web/scripts/capture-screens.mjs`
against the production build (`next start`), the local API and the seeded database. axe ran on
the schedule, the booking sheet and the held status page at every width.

The owner board gets its own review in milestone 5.

## Round 1 (2026-11-14)

| Check | Result | Evidence and fix |
| --- | --- | --- |
| axe | Fail | `label-content-name-mismatch` (serious) on the day buttons: visible "Min 15 Nov", accessible name "Minggu, 15 November 2026", so a voice-control user saying what they see misses. Removed the `aria-label`; the name is now the visible text. `region` (moderate): the demo byline sat outside every landmark. It is now an `aside` labelled "Tentang demo ini". |
| Loading state | Fail | At 375 px the skeleton's third block dropped into the time column. The skeleton now nests its court blocks like the real rows. |
| Error state | Fail (capture) | The capture waited for any `role="alert"` and matched the Next.js route announcer, so the screenshot showed a blank schedule. The script now waits for the error text. |
| Booking not found | Fail | A code of the wrong length fails the API's format check (422) and showed "Periksa nama dan nomor WhatsApp", which is about the booking form, not the code. 422 now shows the same "tidak ditemukan" view as 404, with a test. The "Lihat jam kosong" link was browser-default blue; it is now the page's action style. |
| Status page | Fail | No way back to the schedule and no time zone on the time. Added the venue name as a link home and "WIB" after the time. |
| 375 px | Fail | "07.00–23.00 WIB" broke across two lines; it no longer wraps. |
| 1440 px | Fail | The day strip showed a sliver of the 14th day. The page is now 72rem wide so all 14 days fit on a laptop; phones still scroll the strip. |
| Anti-slop list | Pass | No gradients except the deposit-pending stripes the design brief calls for, no shadows, no card grid, no icons beyond the two profile logos, no all-caps labels or middle-dot meta text. |
| Copy | Pass | Sentence case; buttons name the action ("Bayar DP Rp40.000", "Pilih jam lain", "Muat ulang jadwal"); errors say what failed and what to do. |
| Keyboard | Pass | Focus ring visible on the day strip; the sheet is a native modal `dialog`, so focus stays inside and Escape closes it. |

## Round 2

| Check | Result | Evidence and fix |
| --- | --- | --- |
| axe | Fail | `label-content-name-mismatch` (serious) on the court buttons: the `aria-label` "Lapangan 1, 13.00, 60.000 rupiah per jam" did not contain the visible "Rp60.000". The name now starts with the visible text and a visually hidden suffix adds "per jam, pukul 13.00". |
| 1440 px | Fail | The wider page left the footer on a narrower column. The footer now uses the same 72rem width. |
| Loading state | Fail | On small screens the skeleton's court blocks were wider than real court buttons. They now use the same 6.5rem width. |
| Everything else | Pass | Error, empty, full, held, confirmed, expired and not-found states read correctly at all three widths. |

## Round 3

axe: 0 violations across the 9 checked page and width combinations.

Lighthouse 13.6.0, default mobile emulation with throttling, production build on localhost:

| Page | Performance | Accessibility | Best practices | SEO | LCP | CLS |
| --- | --- | --- | --- | --- | --- | --- |
| Schedule (`/`) | 100 | 100 | 100 | 100 | 0.9 s | 0 |
| Booking status (`/booking/{code}`, held) | 96 | 100 | 100 | 100 | 2.7 s | 0 |

The first Lighthouse run found two more problems, both fixed before the numbers above:
- A missing favicon logged a 404 in the console on every page (Best practices 96). Added
  `app/icon.svg`.
- The status page's footer jumped when the booking finished loading (CLS 0.164). The page now
  reserves the height of its tallest state.

The status page's LCP is the booking panel, which the browser fetches after the page loads.
Rendering it on the server would save one round trip; at 2.7 s on throttled mobile it is not
worth a second data path yet.

Also checked by hand in the browser: schedule, booking sheet, hold, the simulated payment page,
and the return to the status page, which showed the booking confirmed.

Final screenshots for the README are in `docs/screenshots/player-*.png`. The demo recording is
made in milestone 7, once the owner board exists, so one recording shows both sides.
