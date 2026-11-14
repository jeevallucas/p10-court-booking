// Package dbtest gives each test its own empty PostgreSQL database with the schema applied, so
// tests exercise the real constraints and never share state.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/jeevallucas/p10-court-booking/api/internal/migrations"
)

// New creates a fresh database for t, applies the migrations, and drops it when t ends.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("DATABASE_URL")
	if adminURL == "" {
		t.Fatal("DATABASE_URL is not set: start PostgreSQL with `docker compose up -d db` " +
			"and set DATABASE_URL as in .env.example")
	}

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL at DATABASE_URL: %v", err)
	}
	defer closeOrLog(t, "admin connection", func() error { return admin.Close(ctx) })

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "create database "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		cleanup, err := pgx.Connect(context.Background(), adminURL)
		if err != nil {
			t.Errorf("connect to drop test database %s: %v", name, err)
			return
		}
		defer closeOrLog(t, "cleanup connection", func() error {
			return cleanup.Close(context.Background())
		})
		if _, err := cleanup.Exec(context.Background(),
			"drop database "+name+" with (force)"); err != nil {
			t.Errorf("drop test database %s: %v", name, err)
		}
	})

	config, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	config.ConnConfig.Database = name
	// Enough connections for the concurrency tests while staying under PostgreSQL's default
	// limit of 100.
	config.MaxConns = 80

	migrationDB := stdlib.OpenDB(*config.ConnConfig)
	defer closeOrLog(t, "migration connection", migrationDB.Close)
	if err := migrations.Up(ctx, migrationDB); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open pool on test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Court inserts a venue and one court of the given sport and returns the court's ID.
func Court(t testing.TB, pool *pgxpool.Pool, sport string) pgtype.UUID {
	t.Helper()
	var courtID pgtype.UUID
	err := pool.QueryRow(context.Background(), `
		with venue_row as (insert into venue (name) values ('Arena Test') returning id)
		insert into court (venue_id, name, sport, sort_order)
		select id, 'Court ' || gen_random_uuid(), $1, 1 from venue_row
		returning id`, sport).Scan(&courtID)
	if err != nil {
		t.Fatalf("insert test court: %v", err)
	}
	return courtID
}

func closeOrLog(t testing.TB, what string, closeFn func() error) {
	if err := closeFn(); err != nil {
		t.Logf("close %s: %v", what, err)
	}
}
