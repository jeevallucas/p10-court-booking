package owner_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
)

const (
	email    = "pemilik@arenakotabaru.test"
	password = "lapangan-enam-2026"
)

func newSessions(t *testing.T) (*owner.Sessions, *pgxpool.Pool) {
	t.Helper()
	pool := dbtest.New(t)
	venue := dbtest.NewVenue(t, pool)
	hash, err := owner.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `insert into owner_account
		(venue_id, email, password_hash, display_name) values ($1, $2, $3, 'Bu Ratna')`,
		venue.ID, email, hash); err != nil {
		t.Fatal(err)
	}
	return &owner.Sessions{Pool: pool, Lifetime: 12 * time.Hour}, pool
}

func TestHashPasswordStoresArgon2idWithAFreshSalt(t *testing.T) {
	first, err := owner.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := owner.HashPassword(password)
	if !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected format %q", first)
	}
	if first == second {
		t.Fatal("two hashes of the same password are equal; the salt is not random")
	}
}

func TestLogInIssuesASessionTheOwnerCanUse(t *testing.T) {
	sessions, _ := newSessions(t)
	ctx := context.Background()
	token, err := sessions.LogIn(ctx, "Pemilik@ArenaKotabaru.test", password)
	if err != nil {
		t.Fatalf("log in with the right password (email case differs): %v", err)
	}
	signedIn, err := sessions.Lookup(ctx, token)
	if err != nil || signedIn.DisplayName != "Bu Ratna" {
		t.Fatalf("lookup: %+v, %v", signedIn, err)
	}
	if err := sessions.LogOut(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Lookup(ctx, token); !errors.Is(err, owner.ErrNoSession) {
		t.Fatalf("after logout: got %v, want ErrNoSession", err)
	}
}

func TestLogInGivesTheSameErrorForAWrongPasswordAndAnUnknownEmail(t *testing.T) {
	sessions, _ := newSessions(t)
	ctx := context.Background()
	if _, err := sessions.LogIn(ctx, email, "salah"); !errors.Is(err, owner.ErrInvalidCredentials) {
		t.Errorf("wrong password: got %v", err)
	}
	if _, err := sessions.LogIn(ctx, "orang.lain@contoh.test", password); !errors.Is(err, owner.ErrInvalidCredentials) {
		t.Errorf("unknown email: got %v", err)
	}
}

func TestAnExpiredSessionIsRefusedAndCleanedUp(t *testing.T) {
	sessions, pool := newSessions(t)
	ctx := context.Background()
	token, err := sessions.LogIn(ctx, email, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "update owner_session set expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Lookup(ctx, token); !errors.Is(err, owner.ErrNoSession) {
		t.Fatalf("expired session: got %v, want ErrNoSession", err)
	}
	if deleted, err := sessions.DeleteExpired(ctx); err != nil || deleted != 1 {
		t.Fatalf("deleted %d, %v; want 1", deleted, err)
	}
}

func TestATokenThatWasNeverIssuedIsRefused(t *testing.T) {
	sessions, _ := newSessions(t)
	for _, token := range []string{"", "bukan-token"} {
		if _, err := sessions.Lookup(context.Background(), token); !errors.Is(err, owner.ErrNoSession) {
			t.Errorf("token %q: got %v, want ErrNoSession", token, err)
		}
	}
}
