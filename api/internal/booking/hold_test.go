package booking_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
)

var evening = time.Date(2026, 11, 20, 19, 0, 0, 0, time.FixedZone("WIB", 7*60*60))

func holdFor(court pgtype.UUID, start time.Time, hours int) booking.HoldRequest {
	return booking.HoldRequest{
		CourtID:       court,
		Start:         start,
		End:           start.Add(time.Duration(hours) * time.Hour),
		CustomerName:  "Sari Wulandari",
		CustomerPhone: "081200001111",
		PriceIDR:      80000,
		DepositIDR:    40000,
	}
}

func TestPlaceHoldReservesTheSlotForTenMinutes(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")

	held, err := booking.PlaceHold(context.Background(), pool, holdFor(court, evening, 1))
	if err != nil {
		t.Fatalf("place hold: %v", err)
	}
	if held.Status != "held" || len(held.Reference) != 8 {
		t.Fatalf("got status %q and reference %q", held.Status, held.Reference)
	}
	remaining := time.Until(held.HoldExpiresAt.Time)
	if remaining < 9*time.Minute || remaining > 11*time.Minute {
		t.Fatalf("hold expires in %v, want about %v", remaining, booking.HoldDuration)
	}
}

func TestAnOverlappingHoldOnTheSameCourtIsRefused(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")
	ctx := context.Background()

	if _, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 2)); err != nil {
		t.Fatal(err)
	}
	// 20.00-21.00 overlaps 19.00-21.00.
	_, err := booking.PlaceHold(ctx, pool, holdFor(court, evening.Add(time.Hour), 1))
	if !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("got %v, want ErrSlotTaken", err)
	}
}

func TestBookingsThatOnlyTouchDoNotOverlap(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")
	ctx := context.Background()

	if _, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1)); err != nil {
		t.Fatal(err)
	}
	// 20.00-21.00 starts exactly when 19.00-20.00 ends.
	if _, err := booking.PlaceHold(ctx, pool, holdFor(court, evening.Add(time.Hour), 1)); err != nil {
		t.Fatalf("touching hold refused: %v", err)
	}
}

func TestTheSameTimeOnAnotherCourtIsFree(t *testing.T) {
	pool := dbtest.New(t)
	first := dbtest.Court(t, pool, "badminton")
	second := dbtest.Court(t, pool, "badminton")
	ctx := context.Background()

	if _, err := booking.PlaceHold(ctx, pool, holdFor(first, evening, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := booking.PlaceHold(ctx, pool, holdFor(second, evening, 1)); err != nil {
		t.Fatalf("other court refused: %v", err)
	}
}

func TestAnExpiredHoldNoLongerBlocksTheSlot(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")
	ctx := context.Background()

	stale, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, pool, stale.ID, "update booking set hold_expires_at = now() - interval '1 second' where id = $1")

	if _, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1)); err != nil {
		t.Fatalf("slot still blocked by an expired hold: %v", err)
	}
	if status := statusOf(t, pool, stale.ID); status != "expired" {
		t.Fatalf("stale hold has status %q, want expired", status)
	}
}

func TestAHoldThatHasNotExpiredStillBlocks(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")
	ctx := context.Background()

	live, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, pool, live.ID, "update booking set hold_expires_at = now() + interval '5 seconds' where id = $1")

	if _, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1)); !errors.Is(err, booking.ErrSlotTaken) {
		t.Fatalf("got %v, want ErrSlotTaken while the first hold is live", err)
	}
}

func TestACancelledBookingFreesTheSlot(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "futsal")
	ctx := context.Background()

	cancelled, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1))
	if err != nil {
		t.Fatal(err)
	}
	backdate(t, pool, cancelled.ID,
		"update booking set status = 'cancelled', hold_expires_at = null where id = $1")

	if _, err := booking.PlaceHold(ctx, pool, holdFor(court, evening, 1)); err != nil {
		t.Fatalf("slot still blocked by a cancelled booking: %v", err)
	}
}

func TestAPeriodThatDoesNotMoveForwardIsRejected(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")

	request := holdFor(court, evening, 1)
	request.End = request.Start
	if _, err := booking.PlaceHold(context.Background(), pool, request); !errors.Is(err, booking.ErrInvalidPeriod) {
		t.Fatalf("got %v, want ErrInvalidPeriod", err)
	}
}

// The guarantee lives in the database: even inserts that skip the application cannot overlap.
func TestTheDatabaseRejectsOverlapWithoutTheApplication(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")
	ctx := context.Background()
	insert := `insert into booking (reference, court_id, period, status, customer_name,
		customer_phone, price_idr, deposit_idr)
		values ($1, $2, tstzrange($3, $4, '[)'), 'confirmed', 'Budi Santoso', '081200002222', 80000, 0)`

	if _, err := pool.Exec(ctx, insert, "DIRECT01", court, evening, evening.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, insert, "DIRECT02", court, evening.Add(30*time.Minute),
		evening.Add(90*time.Minute))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("got %v, want an exclusion violation (23P01)", err)
	}
}

func TestTwoHundredConcurrentHoldsForOneSlotLeaveExactlyOne(t *testing.T) {
	pool := dbtest.New(t)
	court := dbtest.Court(t, pool, "badminton")
	const attempts = 200

	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := booking.PlaceHold(context.Background(), pool, holdFor(court, evening, 1))
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, booking.ErrSlotTaken):
			refused++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || refused != attempts-1 {
		t.Fatalf("%d holds succeeded and %d were refused; want 1 and %d", succeeded, refused, attempts-1)
	}
	var rows int
	if err := pool.QueryRow(context.Background(),
		"select count(*) from booking where court_id = $1", court).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("%d booking rows exist, want 1", rows)
	}
}

func backdate(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, id); err != nil {
		t.Fatal(err)
	}
}

func statusOf(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		"select status from booking where id = $1", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}
