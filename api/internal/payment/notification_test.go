package payment_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/payment"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

const serverKey = "local-development-server-key"

var wib = func() *time.Location {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err)
	}
	return location
}()

func at(hour int) time.Time { return time.Date(2026, 11, 20, hour, 0, 0, 0, wib) }

type fixture struct {
	pool      *pgxpool.Pool
	bookings  *booking.Service
	processor *payment.Processor
	venue     dbtest.Venue
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := dbtest.New(t)
	return fixture{
		pool:      pool,
		bookings:  &booking.Service{Pool: pool, Now: func() time.Time { return at(10) }},
		processor: &payment.Processor{Pool: pool, ServerKey: serverKey, OwnerEmail: "pemilik@arenakotabaru.test"},
		venue:     dbtest.NewVenue(t, pool),
	}
}

func (f fixture) hold(t *testing.T, courtIndex, hour int) booking.View {
	t.Helper()
	held, err := f.bookings.HoldForPlayer(context.Background(), booking.PlayerHold{
		CourtID: f.venue.Badminton[courtIndex], Start: at(hour), Hours: 1,
		CustomerName: "Rizky Pratama", CustomerPhone: "081234567890",
	})
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// signed builds a notification the way the provider would, signed with the server key.
func signed(reference, transactionID, status string, amount int32) payment.Notification {
	statusCode := map[string]string{"settlement": "200", "pending": "201"}[status]
	if statusCode == "" {
		statusCode = "202"
	}
	gross := fmt.Sprintf("%d.00", amount)
	return payment.Notification{
		OrderID: reference, TransactionID: transactionID, TransactionStatus: status,
		StatusCode: statusCode, GrossAmount: gross,
		SignatureKey: payment.Signature(reference, statusCode, gross, serverKey),
	}
}

func (f fixture) apply(t *testing.T, n payment.Notification) payment.Outcome {
	t.Helper()
	payload, _ := json.Marshal(n)
	outcome, err := f.processor.Apply(context.Background(), n, payload)
	if err != nil {
		t.Fatalf("apply %s: %v", n.TransactionStatus, err)
	}
	return outcome
}

func (f fixture) status(t *testing.T, reference string) store.BookingStatus {
	t.Helper()
	view, err := f.bookings.ByReference(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	return view.Status
}

func (f fixture) count(t *testing.T, sql string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestASettlementConfirmsTheHoldAndQueuesOneConfirmation(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)

	if got := f.apply(t, signed(held.Reference, "trx-1", "settlement", held.DepositIdr)); got != payment.Confirmed {
		t.Fatalf("outcome %s, want confirmed", got)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusConfirmed {
		t.Fatalf("booking is %s", got)
	}
	var recipient, body string
	if err := f.pool.QueryRow(context.Background(),
		"select recipient, body from outbox_message").Scan(&recipient, &body); err != nil {
		t.Fatal(err)
	}
	want := "Halo Rizky Pratama, booking Lapangan 1 pada Jumat, 20 November 2026, 19.00–20.00 WIB " +
		"sudah terkonfirmasi. DP Rp40.000 diterima; sisa Rp40.000 dibayar di kasir. Kode booking: " +
		held.Reference + "."
	if recipient != "whatsapp:+6281234567890" || body != want {
		t.Fatalf("message to %s:\n%s\nwant:\n%s", recipient, body, want)
	}
}

func TestReplayingTheSameNotificationTenTimesChangesNothingAfterTheFirst(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	notification := signed(held.Reference, "trx-1", "settlement", held.DepositIdr)

	outcomes := map[payment.Outcome]int{}
	for range 10 {
		outcomes[f.apply(t, notification)]++
	}
	if outcomes[payment.Confirmed] != 1 || outcomes[payment.Duplicate] != 9 {
		t.Fatalf("outcomes %v, want 1 confirmed and 9 duplicates", outcomes)
	}
	if n := f.count(t, "select count(*) from outbox_message"); n != 1 {
		t.Fatalf("%d confirmation messages queued, want 1", n)
	}
	if n := f.count(t, "select count(*) from payment_notification"); n != 1 {
		t.Fatalf("%d notifications recorded, want 1", n)
	}
}

func TestConcurrentReplaysAreAppliedOnce(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	notification := signed(held.Reference, "trx-1", "settlement", held.DepositIdr)
	payload, _ := json.Marshal(notification)

	results := make(chan payment.Outcome, 10)
	for range 10 {
		go func() {
			outcome, err := f.processor.Apply(context.Background(), notification, payload)
			if err != nil {
				t.Errorf("apply: %v", err)
			}
			results <- outcome
		}()
	}
	confirmed := 0
	for range 10 {
		if <-results == payment.Confirmed {
			confirmed++
		}
	}
	if confirmed != 1 || f.count(t, "select count(*) from outbox_message") != 1 {
		t.Fatalf("%d confirmations, %d messages; want 1 and 1", confirmed,
			f.count(t, "select count(*) from outbox_message"))
	}
}

func TestAFailureThatArrivesAfterPaymentDoesNotUndoIt(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	f.apply(t, signed(held.Reference, "trx-1", "pending", held.DepositIdr))
	f.apply(t, signed(held.Reference, "trx-1", "settlement", held.DepositIdr))
	if got := f.apply(t, signed(held.Reference, "trx-1", "expire", held.DepositIdr)); got != payment.Ignored {
		t.Fatalf("late expire: outcome %s, want ignored", got)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusConfirmed {
		t.Fatalf("booking is %s after a late expire, want confirmed", got)
	}
}

func TestAFailedPaymentReleasesTheHold(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	if got := f.apply(t, signed(held.Reference, "trx-1", "cancel", held.DepositIdr)); got != payment.Released {
		t.Fatalf("outcome %s, want released", got)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusCancelled {
		t.Fatalf("booking is %s, want cancelled", got)
	}
	f.hold(t, 0, 19) // the slot is free again
}

func TestALatePaymentTakesTheSlotBackIfNobodyElseHasIt(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	if _, err := f.pool.Exec(context.Background(), `update booking set status = 'expired',
		hold_expires_at = null where id = $1`, held.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.apply(t, signed(held.Reference, "trx-1", "settlement", held.DepositIdr)); got != payment.Confirmed {
		t.Fatalf("outcome %s, want confirmed", got)
	}
}

func TestALatePaymentForASlotSomeoneElseTookAsksTheOwnerToRefund(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	if _, err := f.pool.Exec(context.Background(), `update booking set status = 'expired',
		hold_expires_at = null where id = $1`, held.ID); err != nil {
		t.Fatal(err)
	}
	f.hold(t, 0, 19) // another player holds the freed slot

	if got := f.apply(t, signed(held.Reference, "trx-1", "settlement", held.DepositIdr)); got != payment.RefundNeeded {
		t.Fatalf("outcome %s, want refund_needed", got)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusExpired {
		t.Fatalf("booking is %s, want it to stay expired", got)
	}
	var recipient string
	if err := f.pool.QueryRow(context.Background(),
		"select recipient from outbox_message").Scan(&recipient); err != nil {
		t.Fatal(err)
	}
	if recipient != "pemilik@arenakotabaru.test" {
		t.Fatalf("refund alert went to %s", recipient)
	}
}

func TestATamperedOrUnknownNotificationIsRefused(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	ctx := context.Background()

	forged := signed(held.Reference, "trx-1", "settlement", held.DepositIdr)
	forged.GrossAmount = "1.00" // changed after signing
	if _, err := f.processor.Apply(ctx, forged, nil); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Errorf("forged amount: got %v, want ErrInvalidSignature", err)
	}

	short := signed(held.Reference, "trx-1", "settlement", held.DepositIdr-1000)
	if _, err := f.processor.Apply(ctx, short, nil); !errors.Is(err, payment.ErrAmountMismatch) {
		t.Errorf("signed but short payment: got %v, want ErrAmountMismatch", err)
	}

	unknown := signed("NOTABOOK", "trx-2", "settlement", 40000)
	if _, err := f.processor.Apply(ctx, unknown, nil); !errors.Is(err, payment.ErrUnknownOrder) {
		t.Errorf("unknown order: got %v, want ErrUnknownOrder", err)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusHeld {
		t.Fatalf("booking is %s after refused notifications, want held", got)
	}
}

func TestAPaymentQueuesBehindAnotherWriterOnTheSameCourt(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	notification := signed(held.Reference, "trx-queue", "settlement", held.DepositIdr)
	dbtest.RequireQueuesBehindCourtLock(t, f.pool, f.venue.Badminton[0], func() error {
		payload, _ := json.Marshal(notification)
		_, err := f.processor.Apply(context.Background(), notification, payload)
		return err
	})
}

func TestADeclinedCardKeepsTheHoldSoThePlayerCanPayAgain(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)

	if got := f.apply(t, signed(held.Reference, "trx-1", "deny", held.DepositIdr)); got != payment.Ignored {
		t.Fatalf("deny: outcome %s, want ignored", got)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusHeld {
		t.Fatalf("after a declined card the booking is %s, want still held", got)
	}
	if got := f.apply(t, signed(held.Reference, "trx-2", "settlement", held.DepositIdr)); got != payment.Confirmed {
		t.Fatalf("the second attempt: outcome %s, want confirmed", got)
	}
}

func TestASettlementMustCarryStatusCode200(t *testing.T) {
	f := newFixture(t)
	held := f.hold(t, 0, 19)
	// A genuinely signed failure, with its unsigned status relabelled as a payment.
	forged := signed(held.Reference, "trx-1", "deny", held.DepositIdr)
	forged.TransactionStatus = "settlement"
	payload, _ := json.Marshal(forged)

	if _, err := f.processor.Apply(context.Background(), forged, payload); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Fatalf("got %v, want ErrInvalidSignature", err)
	}
	if got := f.status(t, held.Reference); got != store.BookingStatusHeld {
		t.Fatalf("booking is %s, want still held", got)
	}
}
