package dbtest

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireQueuesBehindCourtLock runs write while another transaction holds the court's row lock,
// and fails unless write waits for that lock and then succeeds. Booking writes take the court
// lock first so that two of them can never deadlock on the exclusion constraint.
func RequireQueuesBehindCourtLock(t testing.TB, pool *pgxpool.Pool, court pgtype.UUID, write func() error) {
	t.Helper()
	ctx := context.Background()
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Rollback(ctx) }()
	if _, err := other.Exec(ctx, "select id from court where id = $1 for no key update", court); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- write() }()
	select {
	case err := <-done:
		t.Fatalf("the write finished (error: %v) while another transaction held the court", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := other.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the write failed once the court was free: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the write did not finish after the court was released")
	}
}
