# Design brief

Two surfaces: a mobile booking flow for players and a schedule board for the owner. Both are in
Indonesian, with WIB times written the Indonesian way (19.00) and Rupiah amounts (Rp80.000).

## Users and context
- Players book on a phone held in one hand, often outdoors or between games, in under a minute.
  They know the venue; they want to see which courts are free tonight and what it costs.
- The owner works the front desk on a laptop or a tablet on the counter, glancing at the board
  between customers, for hours at a time. Walk-ins arrive while the phone keeps buzzing.

## Top 3 tasks, in their words
1. Player: "Lapangan mana yang masih kosong jam 7 malam ini, dan berapa harganya?" (Which court is
   free at 7 tonight, and how much?)
2. Player: "Pesan dan bayar DP sekarang, tanpa kirim bukti transfer." (Book and pay the deposit
   now, without sending a transfer screenshot.)
3. Owner: "Siapa main di lapangan 3 jam 8, sudah bayar belum, dan bisa saya geser?" (Who plays on
   court 3 at 8, have they paid, and can I move them?)

## Feel
Practical, local, sporty. Rejected: playful. The owner is running a business at a busy desk; the
board should read like the scoreboard on the hall wall, not like a game.

## References
- AYO Indonesia (venue booking). Take: a slot grid with the price on each slot, Indonesian copy.
  Avoid: promotion banners and card-heavy listings around the grid.
- Playtomic (court booking). Take: the date strip and the two-tap court and time choice. Avoid:
  the social feed and the account wall before booking.
- Fresha calendar (salon schedule). Take: one column per resource, drag to move, a side panel for
  the selected appointment. Avoid: pastel blocks whose colours carry no meaning.

## Typography
- Archivo, one family, using its width axis as the expressive device. Times on the board and on
  slot buttons are set wide and heavy (width 125, weight 700, tabular figures), like the digits on
  a hall scoreboard. Names, labels, and body text use normal width (100), weight 400 and 600.
- No other project in this portfolio uses Archivo.
- Scale (ratio about 1.25): 13 px small labels, 16 px body, 20 px section titles, 25 px page title,
  32 px wide times on the player's slot buttons. Line height 1.4 for body, 1.1 for times.
- Sentence case everywhere; no all-caps labels.

## Palette
| Name | Hex | Role | Contrast |
| --- | --- | --- | --- |
| Ink | #16201B | Text, primary buttons, selected outline | 16.7:1 on white |
| Shuttle white | #F4F6F1 | Board and page background | - |
| Court green | #14684A | Confirmed booking (deposit paid or paid at desk); white text on it | 6.8:1 |
| Deposit amber | #F6C453 fill, #8A5300 text and border | Held, waiting for deposit; fill has diagonal stripes | 10.3:1 ink on fill |
| Walk-in slate | #35546E | Walk-in created by the owner; white text on it | 7.9:1 |
| No-show grey | #D5D9D6 fill | No-show; name struck through | 11.7:1 ink on fill |
| Conflict red | #B42318 | Conflict outline and error text only | 6.6:1 on white |

Muted text is #55605A (6.0:1 on shuttle white). Free slots are white with a 1 px #C9CFC9 border.

Each state also has a text label inside its block (Terkonfirmasi, Menunggu DP, Walk-in, Tidak datang; "Lunas" was dropped because a confirmed booking may still owe the rest at the desk, see docs/decisions.md), and
the held state has stripes, so colour is never the only signal. The two light fills (amber, grey)
are below 3:1 against the background, so their blocks carry a 2 px border in the darker tone. A
palette validator found the four state colours clearly distinct for normal and colour-blind vision
(smallest difference 34.5, amber against green, protan simulation).

## Density
- Owner board at 1366 px: six court columns and 07.00 to 23.00 in 30-minute rows fit one screen
  height with scrolling only for the evening. Each block shows name, time, and state; the side
  panel shows the rest.
- Player flow: one day at a time; each hour is one row of court buttons.

## Layout
```
Player, 375 px                      Owner board, 1366 px
| Arena Kotabaru               |    | < Sab 14 Nov >  Hari ini   [Tambah walk-in]           |
| [Sab 14][Min 15][Sen 16] ... |    |       Lap 1  Lap 2  Lap 3  Lap 4 | Futsal A  Futsal B  | Panel   |
| Badminton | Futsal           |    | 19.00 [Budi ][     ][Sari  ][   ] | [Tim Gejayan     ] | Sari W. |
| 19.00                        |    | 19.30 [Lunas][     ][Menun.][   ] | [Lunas           ] | 0812... |
|  [Lap 1 Rp80.000][Lap 3 ...] |    | 20.00 [     ][Walk ][      ][   ] | [                ] | Geser   |
| 20.00                        |    |                                    |                    | Batal   |
|  [Lap 2 Rp80.000]            |
```
Content is left-aligned. The player flow is one column; the owner board is a grid with the time
column fixed on the left and the side panel fixed on the right.

## One bold element
The time column: wide, heavy Archivo numerals (19.00, 19.30) on the board's left edge and on each
of the player's slot buttons, read across the room like a scoreboard. Everything else is flat and
quiet: square-cornered blocks (2 px radius), no shadows, white court-line rules between columns.

## Avoid list for this project
- Rounded cards with soft shadows for bookings or slots; a pastel calendar look.
- Sport emoji or clip-art shuttlecocks as icons.
- A landing page with a hero and three feature cards before the booking grid; the player lands on
  today's free slots.
- ALL-CAPS labels, middle-dot metadata, arrows on every link.
- Animations on every block; motion only when a booking is moved or confirmed.

## Author credit
The player pages and the owner board carry a slim footer: "© 2026 Putu Jeevallucas Jnanamaitriya
Surya Gautama", the GitHub and LinkedIn logos, and "Arena Kotabaru adalah venue fiktif" (a
fictional venue). The demo's landing page has the byline.

## Review against the frontend-design process
- The obvious plan was a SaaS calendar: rounded pastel cards with shadows, a colour per court.
  Changed: colour encodes booking state only, blocks are flat and square, and courts are told
  apart by their column, not their colour.
- The obvious sporty choice was a bright green with a neon accent on a dark background (a
  generated-design default). Changed: a light shuttle-white board with a deep court green, because
  the owner reads it for hours at a front desk in daylight.
- The obvious type pairing was a display face plus a neutral sans. Changed: one family, with
  Archivo's width axis doing the work, because the scoreboard numerals are the thing worth
  remembering and everything else should stay out of the way.
