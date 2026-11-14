package owner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

var (
	// ErrInvalidCredentials covers both an unknown email and a wrong password, so the response
	// does not reveal which accounts exist.
	ErrInvalidCredentials = errors.New("email or password is wrong")
	// ErrNoSession means the token is missing, unknown, or expired.
	ErrNoSession = errors.New("no valid owner session")
)

// Owner is the signed-in account.
type Owner struct {
	ID          pgtype.UUID
	VenueID     pgtype.UUID
	DisplayName string
}

// Sessions issues and checks owner sessions.
type Sessions struct {
	Pool     *pgxpool.Pool
	Lifetime time.Duration
}

// A real hash to verify against when the email is unknown, so a login for a missing account
// takes as long as one for a real account.
var unknownAccountHash = mustHash("not a real password")

func mustHash(password string) string {
	hash, err := HashPassword(password)
	if err != nil {
		panic(err)
	}
	return hash
}

// LogIn checks the password and returns a new session token for the cookie.
func (s *Sessions) LogIn(ctx context.Context, email, password string) (string, error) {
	q := store.New(s.Pool)
	account, err := q.GetOwnerByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = verifyPassword(unknownAccountHash, password)
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("find owner: %w", err)
	}
	ok, err := verifyPassword(account.PasswordHash, password)
	if err != nil {
		return "", fmt.Errorf("check password for %s: %w", email, err)
	}
	if !ok {
		return "", ErrInvalidCredentials
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := q.InsertSession(ctx, store.InsertSessionParams{
		TokenHash: tokenHash(token),
		OwnerID:   account.ID,
		Lifetime:  pgtype.Interval{Microseconds: s.Lifetime.Microseconds(), Valid: true},
	}); err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}
	return token, nil
}

// Lookup returns the owner a session token belongs to.
func (s *Sessions) Lookup(ctx context.Context, token string) (Owner, error) {
	if token == "" {
		return Owner{}, ErrNoSession
	}
	row, err := store.New(s.Pool).GetSessionOwner(ctx, tokenHash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Owner{}, ErrNoSession
	}
	if err != nil {
		return Owner{}, fmt.Errorf("look up session: %w", err)
	}
	return Owner{ID: row.ID, VenueID: row.VenueID, DisplayName: row.DisplayName}, nil
}

// LogOut ends the session; an unknown token is not an error.
func (s *Sessions) LogOut(ctx context.Context, token string) error {
	if err := store.New(s.Pool).DeleteSession(ctx, tokenHash(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpired removes sessions past their expiry and reports how many.
func (s *Sessions) DeleteExpired(ctx context.Context) (int64, error) {
	deleted, err := store.New(s.Pool).DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return deleted, nil
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
