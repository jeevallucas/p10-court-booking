package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/seed"
)

func TestSeedingTwiceResetsTheScheduleWithoutDuplicatingTheVenue(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	wib, _ := time.LoadLocation("Asia/Jakarta")
	options := seed.Options{
		Now:           time.Date(2026, 11, 20, 14, 0, 0, 0, wib),
		Days:          14,
		OwnerPassword: "demo-password",
		RandomSeed:    20261120,
	}

	first, err := seed.Run(ctx, pool, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := seed.Run(ctx, pool, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.Bookings < 200 || first.Bookings != second.Bookings {
		t.Fatalf("seeded %d then %d bookings; want the same number, at least 200", first.Bookings,
			second.Bookings)
	}

	var venues, courts, bookings int
	if err := pool.QueryRow(ctx, `select (select count(*) from venue), (select count(*) from court),
		(select count(*) from booking)`).Scan(&venues, &courts, &bookings); err != nil {
		t.Fatal(err)
	}
	if venues != 1 || courts != 6 || bookings != second.Bookings {
		t.Fatalf("%d venues, %d courts, %d bookings", venues, courts, bookings)
	}

	// The schedule leaves room to book: a player on the seed day can still find free evening slots.
	service := &booking.Service{Pool: pool, Now: func() time.Time { return options.Now }}
	_, slots, err := service.Availability(ctx, options.Now, "badminton")
	if err != nil {
		t.Fatal(err)
	}
	free := 0
	for _, s := range slots {
		if s.Available {
			free++
		}
	}
	if free == 0 || free == len(slots) {
		t.Fatalf("%d of %d badminton slots are free on the seed day; want some but not all", free, len(slots))
	}

	sessions := &owner.Sessions{Pool: pool, Lifetime: time.Hour}
	if _, err := sessions.LogIn(ctx, seed.OwnerEmail, "demo-password"); err != nil {
		t.Fatalf("demo owner cannot log in: %v", err)
	}
}

func TestOnlyFinishedBookingsAreNoShowsAndNoUpcomingBookingIsAWalkInBeyondToday(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	wib, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Date(2026, 11, 20, 14, 0, 0, 0, wib)
	if _, err := seed.Run(ctx, pool, seed.Options{Now: now, Days: 14, OwnerPassword: "x", RandomSeed: 7}); err != nil {
		t.Fatal(err)
	}
	var futureNoShows, laterWalkIns int
	if err := pool.QueryRow(ctx, `select
		count(*) filter (where status = 'no_show' and upper(period) > $1),
		count(*) filter (where status = 'walk_in' and lower(period) > $1 + interval '3 hours')
		from booking`, now).Scan(&futureNoShows, &laterWalkIns); err != nil {
		t.Fatal(err)
	}
	if futureNoShows != 0 || laterWalkIns != 0 {
		t.Fatalf("%d no-shows that have not ended, %d walk-ins more than 3 hours ahead", futureNoShows, laterWalkIns)
	}
}
