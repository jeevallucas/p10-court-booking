// Package seed fills the database with the fictional venue Arena Kotabaru and a realistic two
// weeks of bookings, so the demo never looks empty. Every name and number in it is invented.
package seed

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

const (
	VenueName  = "Arena Kotabaru"
	OwnerEmail = "demo@arenakotabaru.test"
)

// Options controls one seeding run. Now is the moment the schedule is built around: bookings
// before it are finished (some as no-shows), bookings after it are upcoming.
type Options struct {
	Now           time.Time
	Days          int
	OwnerPassword string
	RandomSeed    uint64
}

// Summary reports what a run created.
type Summary struct {
	Bookings int
}

var courts = []struct {
	name, sport string
}{
	{"Lapangan 1", "badminton"}, {"Lapangan 2", "badminton"}, {"Lapangan 3", "badminton"},
	{"Lapangan 4", "badminton"}, {"Futsal A", "futsal"}, {"Futsal B", "futsal"},
}

var prices = []struct {
	sport      string
	from, to   int // hours
	hourlyRate int32
}{
	{"badminton", 7, 17, 60000}, {"badminton", 17, 23, 80000},
	{"futsal", 7, 17, 150000}, {"futsal", 17, 23, 200000},
}

var players = []string{
	"Rizky Pratama", "Dewi Lestari", "Agus Setiawan", "Made Wirawan", "Ni Luh Ayu Saraswati",
	"Yohanes Sitompul", "Maria Simanjuntak", "Fadli Rahman", "Nurul Hidayah", "Kevin Tanoto",
	"Felicia Wijaya", "Hendra Gunawan", "Rina Marlina", "Bayu Nugroho", "Putri Anggraini",
	"Yusuf Maulana", "Ayu Kartika", "Andreas Hutapea", "Lusi Manurung", "Gilang Ramadhan",
	"Teddy Halim", "Wulan Sari", "Arif Budiman", "Fitri Handayani", "Danang Prasetyo",
	"Retno Palupi", "Joko Susilo", "Intan Permatasari", "Galih Saputra", "Rahmawati Siregar",
}

var teams = []string{
	"Tim Seturan", "Futsal Pogung", "Alumni SMA 3", "Karang Taruna Klitren", "Tim Babarsari",
	"Mahasiswa Teknik Sipil", "Bapak-bapak Sabtu Pagi", "Warga Demangan", "Tim Jakal Atas",
	"Komunitas Futsal Kotabaru",
}

// Run resets the schedule and seeds it in one transaction, so the demo is never half-seeded.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) (Summary, error) {
	if opts.OwnerPassword == "" {
		return Summary{}, errors.New("the demo owner password is empty")
	}
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return Summary{}, err
	}
	passwordHash, err := owner.HashPassword(opts.OwnerPassword)
	if err != nil {
		return Summary{}, err
	}

	var summary Summary
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		venueID, err := ensureVenue(ctx, q)
		if err != nil {
			return err
		}
		if err := q.UpsertOwner(ctx, store.UpsertOwnerParams{
			VenueID: venueID, Email: OwnerEmail, PasswordHash: passwordHash,
			DisplayName: "Bu Ratna (demo)",
		}); err != nil {
			return fmt.Errorf("create demo owner: %w", err)
		}
		if err := q.ResetDemoBookings(ctx); err != nil {
			return fmt.Errorf("clear bookings: %w", err)
		}
		summary.Bookings, err = seedBookings(ctx, q, venueID, opts, location)
		return err
	})
	return summary, err
}

func ensureVenue(ctx context.Context, q *store.Queries) (pgtype.UUID, error) {
	venueID, err := q.FindVenueByName(ctx, VenueName)
	if err == nil {
		return venueID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, fmt.Errorf("find venue: %w", err)
	}
	if venueID, err = q.InsertVenue(ctx, VenueName); err != nil {
		return pgtype.UUID{}, fmt.Errorf("create venue: %w", err)
	}
	for i, c := range courts {
		if err := q.InsertCourt(ctx, store.InsertCourtParams{
			VenueID: venueID, Name: c.name, Sport: c.sport, SortOrder: int32(i + 1),
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("create %s: %w", c.name, err)
		}
	}
	for _, p := range prices {
		if err := q.InsertPriceRule(ctx, store.InsertPriceRuleParams{
			VenueID: venueID, Sport: p.sport, HourlyPriceIdr: p.hourlyRate,
			StartsAt: pgtype.Time{Microseconds: int64(p.from) * int64(time.Hour/time.Microsecond), Valid: true},
			EndsAt:   pgtype.Time{Microseconds: int64(p.to) * int64(time.Hour/time.Microsecond), Valid: true},
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("create price rule: %w", err)
		}
	}
	return venueID, nil
}

func seedBookings(ctx context.Context, q *store.Queries, venueID pgtype.UUID, opts Options,
	location *time.Location) (int, error) {
	courtRows, err := q.ListCourts(ctx, venueID)
	if err != nil {
		return 0, fmt.Errorf("list courts: %w", err)
	}
	var rules []booking.PriceRule
	for _, p := range prices {
		rules = append(rules, booking.PriceRule{Sport: p.sport, StartsMinute: p.from * 60,
			EndsMinute: p.to * 60, HourlyPriceIDR: p.hourlyRate})
	}
	rng := rand.New(rand.NewPCG(opts.RandomSeed, opts.RandomSeed^0x9e3779b97f4a7c15))
	now := opts.Now.In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)

	created := 0
	for day := range opts.Days {
		date := today.AddDate(0, 0, day)
		for _, court := range courtRows {
			for hour := 7; hour < 23; {
				if rng.Float64() >= occupancy(court.Sport, date.Weekday(), hour) {
					hour++
					continue
				}
				hours := 1
				if hour+2 <= 23 && rng.Float64() < twoHourShare(court.Sport) {
					hours = 2
				}
				start := date.Add(time.Duration(hour) * time.Hour)
				end := start.Add(time.Duration(hours) * time.Hour)
				if err := insertBooking(ctx, q, rng, court, rules, start, end, now); err != nil {
					return created, err
				}
				created++
				hour += hours
			}
		}
	}
	return created, nil
}

// occupancy is the chance that a free hour is booked: busy evenings and weekend mornings, quiet
// weekday middays. The figures are invented to make the schedule look like a real hall.
func occupancy(sport string, weekday time.Weekday, hour int) float64 {
	weekend := weekday == time.Saturday || weekday == time.Sunday
	if sport == "futsal" {
		switch {
		case weekend && hour < 12:
			return 0.45
		case weekend && hour < 16:
			return 0.35
		case weekend:
			return 0.75
		case hour < 15:
			return 0.08
		case hour < 18:
			return 0.35
		case hour < 22:
			return 0.8
		default:
			return 0.5
		}
	}
	switch {
	case weekend && hour < 12:
		return 0.6
	case weekend && hour < 16:
		return 0.35
	case weekend && hour < 22:
		return 0.7
	case weekend:
		return 0.3
	case hour < 9:
		return 0.35
	case hour < 16:
		return 0.12
	case hour < 18:
		return 0.4
	case hour < 22:
		return 0.75
	default:
		return 0.35
	}
}

func twoHourShare(sport string) float64 {
	if sport == "futsal" {
		return 0.5
	}
	return 0.3
}

func insertBooking(ctx context.Context, q *store.Queries, rng *rand.Rand, court store.ListCourtsRow,
	rules []booking.PriceRule, start, end, now time.Time) error {
	price, err := booking.Price(rules, court.Sport, start, end)
	if err != nil {
		return err
	}
	status := store.BookingStatusConfirmed
	switch roll := rng.Float64(); {
	case !end.After(now) && roll < 0.06:
		status = store.BookingStatusNoShow
	case !end.After(now) && roll < 0.20:
		status = store.BookingStatusWalkIn
	case start.Before(now.Add(3*time.Hour)) && start.After(now.Add(-time.Hour)) && roll < 0.25:
		status = store.BookingStatusWalkIn // today, at the desk
	}
	deposit := booking.Deposit(price)
	if status == store.BookingStatusWalkIn {
		deposit = 0
	}
	customer := players[rng.IntN(len(players))]
	if court.Sport == "futsal" {
		customer = teams[rng.IntN(len(teams))]
	}
	createdAt := start.Add(-time.Duration(1+rng.IntN(72)) * time.Hour)
	if status == store.BookingStatusWalkIn || createdAt.After(now) {
		createdAt = minTime(start, now)
	}
	reference, err := booking.NewReference()
	if err != nil {
		return err
	}
	if err := q.InsertSeedBooking(ctx, store.InsertSeedBookingParams{
		Reference:     reference,
		CourtID:       court.ID,
		Period:        booking.Period(start, end),
		Status:        status,
		CustomerName:  customer,
		CustomerPhone: fmt.Sprintf("+628000000%04d", rng.IntN(10000)),
		PriceIdr:      price,
		DepositIdr:    deposit,
		CreatedAt:     pgtype.Timestamptz{Time: createdAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("insert booking on %s at %s: %w", court.Name, start.Format("2006-01-02 15.04"), err)
	}
	return nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
