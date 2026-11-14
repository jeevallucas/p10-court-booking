// Package booking holds the rules for reserving a court: holds, confirmations, and their
// lifecycle. Overlap is prevented by the database's exclusion constraint, not by locks here.
package booking

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

// HoldDuration is how long a player has to pay the deposit before the slot is released.
const HoldDuration = 10 * time.Minute

// ErrSlotTaken means another booking already occupies the court for part of the requested time.
var ErrSlotTaken = errors.New("the court is already booked for part of this time")

// ErrInvalidPeriod means the requested end is not after the start.
var ErrInvalidPeriod = errors.New("the booking must end after it starts")

// HoldRequest is a player's request to reserve a court while they pay the deposit.
type HoldRequest struct {
	CourtID       pgtype.UUID
	Start, End    time.Time
	CustomerName  string
	CustomerPhone string
	PriceIDR      int32
	DepositIDR    int32
}

// PlaceHold reserves the court for HoldDuration. Exactly one of any set of concurrent requests for
// overlapping time on the same court succeeds; the rest get ErrSlotTaken.
func PlaceHold(ctx context.Context, pool *pgxpool.Pool, req HoldRequest) (store.Booking, error) {
	if !req.End.After(req.Start) {
		return store.Booking{}, ErrInvalidPeriod
	}
	reference, err := newReference()
	if err != nil {
		return store.Booking{}, err
	}
	period := Period(req.Start, req.End)

	var held store.Booking
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		queries := store.New(tx)
		// A hold whose time ran out still counts for the constraint until it is marked expired.
		if _, err := queries.ExpireOverlappingHolds(ctx, store.ExpireOverlappingHoldsParams{
			CourtID: req.CourtID,
			Period:  period,
		}); err != nil {
			return fmt.Errorf("expire stale holds: %w", err)
		}
		held, err = queries.InsertHold(ctx, store.InsertHoldParams{
			Reference:     reference,
			CourtID:       req.CourtID,
			Period:        period,
			CustomerName:  req.CustomerName,
			CustomerPhone: req.CustomerPhone,
			PriceIdr:      req.PriceIDR,
			DepositIdr:    req.DepositIDR,
			HoldFor:       pgtype.Interval{Microseconds: HoldDuration.Microseconds(), Valid: true},
		})
		return err
	})
	if isOverlapViolation(err) {
		return store.Booking{}, ErrSlotTaken
	}
	if err != nil {
		return store.Booking{}, fmt.Errorf("place hold: %w", err)
	}
	return held, nil
}

// Period builds the half-open [start, end) range the booking table stores.
func Period(start, end time.Time) pgtype.Range[pgtype.Timestamptz] {
	return pgtype.Range[pgtype.Timestamptz]{
		Lower:     pgtype.Timestamptz{Time: start, Valid: true},
		Upper:     pgtype.Timestamptz{Time: end, Valid: true},
		LowerType: pgtype.Inclusive,
		UpperType: pgtype.Exclusive,
		Valid:     true,
	}
}

func isOverlapViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23P01" &&
		pgErr.ConstraintName == "booking_no_overlap"
}

// Crockford's base32 alphabet leaves out I, L, O, and U, so a reference read aloud over the
// phone is not mistaken for another.
const referenceAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newReference returns an 8-character booking code: 40 random bits, so a collision among a
// venue's bookings is negligible, and the unique constraint still catches one.
func newReference() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate booking reference: %w", err)
	}
	code := make([]byte, len(random))
	for i, b := range random {
		code[i] = referenceAlphabet[int(b)%len(referenceAlphabet)]
	}
	return string(code), nil
}
