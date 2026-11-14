// Command seed resets the demo: the fictional venue, its demo owner, and two weeks of bookings
// around the current time. The nightly workflow runs it against the demo database.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/jeevallucas/p10-court-booking/api/internal/migrations"
	"github.com/jeevallucas/p10-court-booking/api/internal/seed"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is not set; copy .env.example to .env or export it")
	}
	password := os.Getenv("DEMO_OWNER_PASSWORD")
	if password == "" {
		return errors.New("DEMO_OWNER_PASSWORD is not set; it becomes the demo owner's password")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to the database: %w", err)
	}
	defer pool.Close()
	migrationDB := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(ctx, migrationDB); err != nil {
		return err
	}
	if err := migrationDB.Close(); err != nil {
		return err
	}

	now := time.Now()
	summary, err := seed.Run(ctx, pool, seed.Options{
		Now:           now,
		Days:          14,
		OwnerPassword: password,
		// A new day gives a new schedule; the same day reproduces the same one.
		RandomSeed: uint64(now.Year()*10000 + int(now.Month())*100 + now.Day()),
	})
	if err != nil {
		return err
	}
	fmt.Printf("seeded %s with %d bookings; owner login %s\n", seed.VenueName, summary.Bookings,
		seed.OwnerEmail)
	return nil
}
