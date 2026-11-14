# Brief

The owner of a six-court badminton and futsal venue uses this system to run the day from one
schedule board, deciding which slot goes to whom, whether to take a walk-in, and what to do about a
no-show, while players book and pay a deposit from their phones without a double booking.

## Users
The venue is fictional: Arena Kotabaru in Yogyakarta, with four badminton courts and two futsal
courts, open 07:00 to 23:00 WIB. The people and habits below are assumptions used to scope the
work.

- **The owner**, at the front desk on a laptop or tablet, many times a day. Today the schedule
  lives in WhatsApp chats and a paper book, and deposits are bank transfers checked by hand against
  screenshots.
- **Players**, on phones, booking for themselves or a group. They want to see free slots, pick
  one, and pay the deposit in under a minute.

## Decisions the system supports
- Owner: which slot goes to whom; whether to accept a walk-in into a gap; whether to mark a
  booking as a no-show and release the court; where to move a booking when a court is unavailable.
- Player: which court and time to book, and whether to pay the deposit now.

## Constraints
- A slot can never be booked twice, even when many players try at the same moment.
- A player's slot is held while they pay; an unpaid hold must expire on its own.
- No real money moves. Payment uses a fake provider with the Midtrans sandbox's signed
  notification contract, because the sandbox account could not be opened (docs/decisions.md).
  Payment notifications can arrive late, twice, or out of order.
- No real SMS or WhatsApp messages are sent; confirmations go to a local mail catcher in
  development and to logs in the demo.
- The demo runs on free tiers without a credit card: Vercel Hobby for the web app and the API, and
  Neon's free Postgres (1 GB storage per project). Render was the first choice for the API but
  asks for a card even for free services (docs/decisions.md).
- The interface is in Indonesian, with WIB times and Rupiah amounts; code and documents are in
  English.

## Success metrics
- At least 200 concurrent attempts to book the same slot produce exactly one booking; the result
  and the p95 latency are reported in the README.
- Replaying the same payment notification 10 times changes nothing after the first.
- End-to-end tests cover book, pay, confirmed, and an owner drag-to-reschedule that hits a
  conflict.
- Lighthouse accessibility score of at least 95 on the player flow and the owner board.

## Non-goals
A multi-venue marketplace, native mobile apps, real SMS or WhatsApp sending, automated refunds,
memberships, and loyalty features.

## Data
All data is synthetic: a seed program in the repository creates the venue, its courts and prices,
and two weeks of bookings with fictional Indonesian names and phone numbers. No real person,
business, or payment appears anywhere.
