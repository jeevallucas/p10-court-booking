package booking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

var wib = mustLocation("Asia/Jakarta")

func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

// local is a time on the test day, 20 November 2026, in the venue's zone.
func local(hour, minute int) time.Time {
	return time.Date(2026, 11, 20, hour, minute, 0, 0, wib)
}

func newService(t *testing.T, now time.Time) (*booking.Service, dbtest.Venue) {
	t.Helper()
	pool := dbtest.New(t)
	venue := dbtest.NewVenue(t, pool)
	return &booking.Service{Pool: pool, Now: func() time.Time { return now }}, venue
}

func playerHold(court pgtype.UUID, start time.Time, hours int) booking.PlayerHold {
	return booking.PlayerHold{
		CourtID: court, Start: start, Hours: hours,
		CustomerName: "Rizky Pratama", CustomerPhone: "0812-3456-7890",
	}
}

func TestAvailabilityListsEveryHourAndMarksTakenAndPastSlots(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	ctx := context.Background()
	if _, err := service.HoldForPlayer(ctx, playerHold(venue.Badminton[0], local(19, 0), 1)); err != nil {
		t.Fatal(err)
	}

	courts, slots, err := service.Availability(ctx, local(0, 0), "badminton")
	if err != nil {
		t.Fatal(err)
	}
	if len(courts) != 4 || len(slots) != 4*16 {
		t.Fatalf("got %d courts and %d slots, want 4 and 64 (07.00 to 23.00)", len(courts), len(slots))
	}
	find := func(court pgtype.UUID, hour int) booking.Slot {
		for _, s := range slots {
			if s.CourtID == court && s.Start.Equal(local(hour, 0)) {
				return s
			}
		}
		t.Fatalf("no slot at %d.00", hour)
		return booking.Slot{}
	}
	if find(venue.Badminton[0], 19).Available {
		t.Error("the held 19.00 slot is shown as free")
	}
	if !find(venue.Badminton[1], 19).Available {
		t.Error("19.00 on another court is shown as taken")
	}
	if find(venue.Badminton[1], 9).Available {
		t.Error("09.00 has already started at 10.00 but is shown as free")
	}
	if got := find(venue.Badminton[1], 19).PriceIDR; got != 80000 {
		t.Errorf("evening price %d, want 80000", got)
	}
}

func TestAPlayerHoldIsPricedByTheServer(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	held, err := service.HoldForPlayer(context.Background(), playerHold(venue.Badminton[0], local(16, 0), 2))
	if err != nil {
		t.Fatal(err)
	}
	if held.PriceIdr != 140000 || held.DepositIdr != 70000 {
		t.Fatalf("price %d deposit %d, want 140000 and 70000", held.PriceIdr, held.DepositIdr)
	}
	if held.CustomerPhone != "+6281234567890" || held.CourtName != "Lapangan 1" {
		t.Fatalf("phone %q court %q", held.CustomerPhone, held.CourtName)
	}
}

func TestAPlayerHoldIsRefusedWhenTheRequestBreaksARule(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	unknownCourt := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	cases := []struct {
		name string
		hold booking.PlayerHold
		want error
	}{
		{"starts at half past", playerHold(venue.Badminton[0], local(19, 30), 1), booking.ErrInvalidRequest},
		{"runs past closing", playerHold(venue.Badminton[0], local(22, 0), 2), booking.ErrOutsideOpeningHours},
		{"before opening", playerHold(venue.Badminton[0], local(6, 0), 1), booking.ErrOutsideOpeningHours},
		{"already started", playerHold(venue.Badminton[0], local(10, 0), 1), booking.ErrInThePast},
		{"three hours", playerHold(venue.Badminton[0], local(19, 0), 3), booking.ErrInvalidRequest},
		{"unknown court", playerHold(unknownCourt, local(19, 0), 1), booking.ErrNotFound},
	}
	for _, c := range cases {
		if _, err := service.HoldForPlayer(context.Background(), c.hold); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
	badPhone := playerHold(venue.Badminton[0], local(19, 0), 1)
	badPhone.CustomerPhone = "021555123"
	if _, err := service.HoldForPlayer(context.Background(), badPhone); !errors.Is(err, booking.ErrInvalidRequest) {
		t.Errorf("landline: got %v, want ErrInvalidRequest", err)
	}
}

func TestAWalkInBlocksTheSlotForPlayers(t *testing.T) {
	service, venue := newService(t, local(18, 50))
	ctx := context.Background()
	walkIn, err := service.CreateWalkIn(ctx, booking.WalkIn{
		CourtID: venue.Badminton[2], Start: local(19, 0), End: local(20, 30), CustomerName: "Pak Hendra",
	})
	if err != nil {
		t.Fatal(err)
	}
	if walkIn.Status != store.BookingStatusWalkIn || walkIn.PriceIdr != 120000 || walkIn.DepositIdr != 0 {
		t.Fatalf("walk-in %+v", walkIn.Booking)
	}
	if _, err := service.HoldForPlayer(ctx, playerHold(venue.Badminton[2], local(20, 0), 1)); !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("got %v, want ErrSlotTaken", err)
	}
}

func TestRescheduleMovesABookingButNotIntoAConflict(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	ctx := context.Background()
	first := mustWalkIn(t, service, venue.Badminton[0], local(19, 0), local(20, 0))
	mustWalkIn(t, service, venue.Badminton[1], local(19, 0), local(20, 0))

	if _, err := service.Reschedule(ctx, first.ID, venue.Badminton[1], local(19, 30), local(20, 30)); !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("into an occupied court: got %v, want ErrSlotTaken", err)
	}
	moved, err := service.Reschedule(ctx, first.ID, venue.Badminton[3], local(20, 0), local(21, 0))
	if err != nil {
		t.Fatalf("into a free court: %v", err)
	}
	if moved.CourtName != "Lapangan 4" || !moved.Period.Lower.Time.Equal(local(20, 0)) {
		t.Fatalf("moved to %s at %v", moved.CourtName, moved.Period.Lower.Time)
	}
	if moved.PriceIdr != first.PriceIdr {
		t.Fatalf("price changed from %d to %d", first.PriceIdr, moved.PriceIdr)
	}
	if _, err := service.Reschedule(ctx, first.ID, venue.Futsal[0], local(20, 0), local(21, 0)); !errors.Is(err, booking.ErrInvalidRequest) {
		t.Fatalf("badminton to futsal: got %v, want ErrInvalidRequest", err)
	}
}

func TestAHoldCannotBeMovedWhileThePlayerIsPaying(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	held, err := service.HoldForPlayer(context.Background(), playerHold(venue.Badminton[0], local(19, 0), 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reschedule(context.Background(), held.ID, venue.Badminton[1], local(19, 0), local(20, 0)); !errors.Is(err, booking.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestStatusChangesFollowTheBookingLifecycle(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	ctx := context.Background()
	held, err := service.HoldForPlayer(ctx, playerHold(venue.Badminton[0], local(19, 0), 1))
	if err != nil {
		t.Fatal(err)
	}

	paid, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusConfirmed)
	if err != nil || paid.Status != store.BookingStatusConfirmed || paid.HoldExpiresAt.Valid {
		t.Fatalf("paid at the desk: %+v, %v", paid.Booking, err)
	}
	if _, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusConfirmed); !errors.Is(err, booking.ErrInvalidTransition) {
		t.Errorf("confirm twice: got %v, want ErrInvalidTransition", err)
	}
	if _, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusNoShow); !errors.Is(err, booking.ErrInvalidTransition) {
		t.Errorf("no-show before 19.00: got %v, want ErrInvalidTransition", err)
	}

	service.Now = func() time.Time { return local(19, 20) }
	if _, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusNoShow); err != nil {
		t.Errorf("no-show after 19.00: %v", err)
	}
	if _, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusCancelled); !errors.Is(err, booking.ErrInvalidTransition) {
		t.Errorf("cancel a no-show: got %v, want ErrInvalidTransition", err)
	}
	if _, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusHeld); !errors.Is(err, booking.ErrInvalidRequest) {
		t.Errorf("set back to held: got %v, want ErrInvalidRequest", err)
	}
	unknown := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	if _, err := service.ChangeStatus(ctx, unknown, store.BookingStatusCancelled); !errors.Is(err, booking.ErrNotFound) {
		t.Errorf("unknown booking: got %v, want ErrNotFound", err)
	}
}

func TestANoShowFreesTheCourtForAWalkIn(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	ctx := context.Background()
	booked := mustWalkIn(t, service, venue.Futsal[0], local(19, 0), local(21, 0))
	service.Now = func() time.Time { return local(19, 15) }
	if _, err := service.ChangeStatus(ctx, booked.ID, store.BookingStatusNoShow); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateWalkIn(ctx, booking.WalkIn{
		CourtID: venue.Futsal[0], Start: local(19, 0), End: local(21, 0), CustomerName: "Tim Gejayan",
	}); err != nil {
		t.Fatalf("walk-in after a no-show: %v", err)
	}
}

func TestTheSweepExpiresOnlyHoldsWhoseTimeIsOver(t *testing.T) {
	service, venue := newService(t, local(10, 0))
	ctx := context.Background()
	stale, err := service.HoldForPlayer(ctx, playerHold(venue.Badminton[0], local(19, 0), 1))
	if err != nil {
		t.Fatal(err)
	}
	live, err := service.HoldForPlayer(ctx, playerHold(venue.Badminton[1], local(19, 0), 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Pool.Exec(ctx,
		"update booking set hold_expires_at = now() - interval '1 second' where id = $1", stale.ID); err != nil {
		t.Fatal(err)
	}

	expired, err := service.ExpireHolds(ctx)
	if err != nil || expired != 1 {
		t.Fatalf("expired %d, %v; want 1", expired, err)
	}
	if view, _ := service.ByReference(ctx, stale.Reference); view.Status != store.BookingStatusExpired {
		t.Errorf("stale hold is %s", view.Status)
	}
	if view, _ := service.ByReference(ctx, live.Reference); view.Status != store.BookingStatusHeld {
		t.Errorf("live hold is %s", view.Status)
	}
}

func mustWalkIn(t *testing.T, service *booking.Service, court pgtype.UUID, start, end time.Time) booking.View {
	t.Helper()
	view, err := service.CreateWalkIn(context.Background(), booking.WalkIn{
		CourtID: court, Start: start, End: end, CustomerName: "Dewi Lestari", CustomerPhone: "081298765432",
	})
	if err != nil {
		t.Fatalf("walk-in: %v", err)
	}
	return view
}

func TestOwnerWritesQueueBehindAnotherWriterOnTheSameCourt(t *testing.T) {
	ctx := context.Background()
	t.Run("walk-in", func(t *testing.T) {
		service, venue := newService(t, local(10, 0))
		dbtest.RequireQueuesBehindCourtLock(t, service.Pool, venue.Badminton[0], func() error {
			_, err := service.CreateWalkIn(ctx, booking.WalkIn{
				CourtID: venue.Badminton[0], Start: local(19, 0), End: local(20, 0), CustomerName: "Joko Susilo",
			})
			return err
		})
	})
	t.Run("reschedule onto the court", func(t *testing.T) {
		service, venue := newService(t, local(10, 0))
		walkIn := mustWalkIn(t, service, venue.Badminton[0], local(19, 0), local(20, 0))
		dbtest.RequireQueuesBehindCourtLock(t, service.Pool, venue.Badminton[1], func() error {
			_, err := service.Reschedule(ctx, walkIn.ID, venue.Badminton[1], local(19, 0), local(20, 0))
			return err
		})
	})
	t.Run("deposit paid at the desk", func(t *testing.T) {
		service, venue := newService(t, local(10, 0))
		held, err := service.HoldForPlayer(ctx, playerHold(venue.Badminton[2], local(19, 0), 1))
		if err != nil {
			t.Fatal(err)
		}
		dbtest.RequireQueuesBehindCourtLock(t, service.Pool, venue.Badminton[2], func() error {
			_, err := service.ChangeStatus(ctx, held.ID, store.BookingStatusConfirmed)
			return err
		})
	})
}
