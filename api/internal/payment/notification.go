// Package payment applies deposit payment notifications to bookings. Notifications follow
// Midtrans's HTTP notification contract (fields and SHA-512 signature); in this project a local
// fake provider sends them, as docs/decisions.md explains.
package payment

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/notify"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

var (
	// ErrInvalidSignature means the notification was not signed with the server key.
	ErrInvalidSignature = errors.New("notification signature does not match")
	// ErrAmountMismatch means the paid amount is not the booking's deposit.
	ErrAmountMismatch = errors.New("paid amount does not match the deposit")
	// ErrUnknownOrder means no booking has the notification's order ID.
	ErrUnknownOrder = errors.New("no booking has this order ID")
)

// Notification holds the fields of a Midtrans HTTP notification that this service reads.
type Notification struct {
	OrderID           string
	TransactionID     string
	TransactionStatus string
	StatusCode        string
	GrossAmount       string // "40000.00"
	FraudStatus       string
	SignatureKey      string
}

// Outcome is what a notification did to its booking.
type Outcome string

const (
	// Confirmed: the deposit was paid and the booking is confirmed.
	Confirmed Outcome = "confirmed"
	// Released: the payment failed or was cancelled, so the hold was released.
	Released Outcome = "released"
	// Ignored: the notification changes nothing (pending, or it arrived after a final state).
	Ignored Outcome = "ignored"
	// RefundNeeded: money arrived for a slot that is no longer the player's.
	RefundNeeded Outcome = "refund_needed"
	// Duplicate: this exact notification was already applied.
	Duplicate Outcome = "duplicate"
)

// Signature is Midtrans's notification signature:
// SHA512(order_id + status_code + gross_amount + server_key), hex-encoded.
func Signature(orderID, statusCode, grossAmount, serverKey string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
	return hex.EncodeToString(sum[:])
}

// Processor applies notifications. OwnerEmail receives refund alerts.
type Processor struct {
	Pool       *pgxpool.Pool
	ServerKey  string
	OwnerEmail string
}

var errReplay = errors.New("notification already recorded")

// Apply verifies a notification and applies it once. Notifications for one booking are applied
// one at a time under a row lock; a replay of the same transaction and status changes nothing.
func (p *Processor) Apply(ctx context.Context, n Notification, payload []byte) (Outcome, error) {
	want := Signature(n.OrderID, n.StatusCode, n.GrossAmount, p.ServerKey)
	if subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(n.SignatureKey))) != 1 {
		return "", ErrInvalidSignature
	}
	// The signature covers status_code but not transaction_status. Midtrans sends 200 with every
	// payment, so a "settlement" with another code is a signed failure that someone relabelled.
	if paid(n) && n.StatusCode != "200" {
		return "", ErrInvalidSignature
	}
	amount, err := parseRupiah(n.GrossAmount)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAmountMismatch, err)
	}

	var outcome Outcome
	err = pgx.BeginFunc(ctx, p.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// The court before the booking: the lock order every booking write follows.
		if err := q.LockCourtOfReference(ctx, n.OrderID); err != nil {
			return fmt.Errorf("lock court: %w", err)
		}
		row, err := q.LockBookingByReference(ctx, n.OrderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownOrder
		}
		if err != nil {
			return fmt.Errorf("lock booking: %w", err)
		}
		if amount != row.Booking.DepositIdr {
			return fmt.Errorf("%w: paid %d, deposit is %d", ErrAmountMismatch, amount,
				row.Booking.DepositIdr)
		}
		view := booking.View{Booking: row.Booking, CourtName: row.CourtName, Sport: row.Sport}

		outcome, err = p.decide(ctx, tx, n, view)
		if err != nil {
			return err
		}
		if _, err := q.InsertNotification(ctx, store.InsertNotificationParams{
			BookingID:         row.Booking.ID,
			TransactionID:     n.TransactionID,
			TransactionStatus: n.TransactionStatus,
			GrossAmountIdr:    amount,
			Outcome:           string(outcome),
			Payload:           payload,
		}); errors.Is(err, pgx.ErrNoRows) {
			// Rolling back undoes anything decide changed; the first delivery already did it.
			return errReplay
		} else if err != nil {
			return fmt.Errorf("record notification: %w", err)
		}
		return nil
	})
	if errors.Is(err, errReplay) {
		return Duplicate, nil
	}
	if err != nil {
		return "", err
	}
	return outcome, nil
}

// decide applies the notification to the locked booking and queues any message it implies.
func (p *Processor) decide(ctx context.Context, tx pgx.Tx, n Notification, view booking.View) (Outcome, error) {
	q := store.New(tx)
	switch {
	case paid(n):
		switch view.Status {
		case store.BookingStatusConfirmed:
			return Ignored, nil
		case store.BookingStatusHeld, store.BookingStatusExpired:
			confirmed, err := p.claim(ctx, tx, view)
			if err != nil {
				return "", err
			}
			if !confirmed {
				return RefundNeeded, notify.Queue(ctx, q, notify.RefundAlert(p.OwnerEmail, view.Notice()))
			}
			return Confirmed, notify.Queue(ctx, q, notify.Confirmation(view.Notice()))
		default: // cancelled by the owner, a no-show, or a walk-in: the deposit has no slot
			return RefundNeeded, notify.Queue(ctx, q, notify.RefundAlert(p.OwnerEmail, view.Notice()))
		}
	case failed(n):
		if view.Status != store.BookingStatusHeld {
			return Ignored, nil // a failure that arrives after payment or expiry changes nothing
		}
		if _, err := q.ReleaseHold(ctx, view.ID); err != nil {
			return "", fmt.Errorf("release hold: %w", err)
		}
		return Released, nil
	default: // pending, authorize, refund: nothing to do automatically
		return Ignored, nil
	}
}

// claim confirms a paid booking. For a hold that expired before the money arrived, it tries to
// take the slot back; a savepoint lets the transaction continue if the slot is gone.
func (p *Processor) claim(ctx context.Context, tx pgx.Tx, view booking.View) (bool, error) {
	if _, err := store.New(tx).ExpireOverlappingHolds(ctx, store.ExpireOverlappingHoldsParams{
		CourtID: view.CourtID, Period: view.Period,
	}); err != nil {
		return false, fmt.Errorf("expire stale holds: %w", err)
	}
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("savepoint: %w", err)
	}
	_, err = store.New(savepoint).ConfirmPaidBooking(ctx, view.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
			return false, fmt.Errorf("roll back to savepoint: %w", rollbackErr)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("confirm booking: %w", err)
	}
	if err := savepoint.Commit(ctx); err != nil {
		return false, fmt.Errorf("release savepoint: %w", err)
	}
	return true, nil
}

func paid(n Notification) bool {
	switch n.TransactionStatus {
	case "settlement":
		return true
	case "capture": // card payments: only an accepted fraud check means money is captured
		return n.FraudStatus == "accept"
	}
	return false
}

// failed reports a payment that is over. A "deny" is not: the player can try another card or
// method while the hold lasts, so it leaves the hold in place.
func failed(n Notification) bool {
	switch n.TransactionStatus {
	case "cancel", "expire", "failure":
		return true
	}
	return false
}

// parseRupiah reads Midtrans's gross_amount ("40000.00") as whole Rupiah.
func parseRupiah(amount string) (int32, error) {
	whole, fraction, _ := strings.Cut(amount, ".")
	if strings.Trim(fraction, "0") != "" {
		return 0, fmt.Errorf("amount %q has a fraction of a Rupiah", amount)
	}
	value, err := strconv.ParseInt(whole, 10, 32)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("amount %q is not a whole number of Rupiah", amount)
	}
	return int32(value), nil
}
