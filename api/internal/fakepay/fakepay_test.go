package fakepay_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/fakepay"
	"github.com/jeevallucas/p10-court-booking/api/internal/httpapi"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/payment"
	"github.com/jeevallucas/p10-court-booking/api/internal/ratelimit"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

const serverKey = "test-server-key"

var wib = func() *time.Location {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err)
	}
	return location
}()

// newProvider runs the API and the fake provider together, as cmd/api does.
func newProvider(t *testing.T) (*httptest.Server, *booking.Service, dbtest.Venue) {
	t.Helper()
	pool := dbtest.New(t)
	venue := dbtest.NewVenue(t, pool)
	bookings := &booking.Service{Pool: pool, Now: func() time.Time {
		return time.Date(2026, 11, 20, 10, 0, 0, 0, wib)
	}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	api, err := httpapi.NewHandler(httpapi.Config{
		Bookings: bookings,
		Sessions: &owner.Sessions{Pool: pool, Lifetime: time.Hour},
		Payments: &payment.Processor{Pool: pool, ServerKey: serverKey,
			OwnerEmail: "pemilik@arenakotabaru.test"},
		PaymentPageURL: "http://web.test/fakepay/",
		Logger:         logger,
		HoldLimiter:    ratelimit.New(1000, 1000),
		LoginLimiter:   ratelimit.New(1000, 1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.Handle("/", api)
	mux.Handle("/fakepay/", fakepay.Handler(fakepay.Config{
		Bookings:        bookings,
		ServerKey:       serverKey,
		NotificationURL: server.URL + "/v1/payments/notifications",
		ReturnURL:       "http://web.test/booking/",
		Client:          server.Client(),
		Logger:          logger,
	}))
	return server, bookings, venue
}

func holdAt(t *testing.T, bookings *booking.Service, venue dbtest.Venue, hour int) booking.View {
	t.Helper()
	held, err := bookings.HoldForPlayer(context.Background(), booking.PlayerHold{
		CourtID: venue.Badminton[0], Start: time.Date(2026, 11, 20, hour, 0, 0, 0, wib), Hours: 1,
		CustomerName: "Rizky Pratama", CustomerPhone: "081234567890",
	})
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func submit(t *testing.T, server *httptest.Server, reference, action string) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(server.URL+"/fakepay/"+reference, url.Values{"action": {action}})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestThePaymentPageShowsTheDeposit(t *testing.T) {
	server, bookings, venue := newProvider(t)
	held := holdAt(t, bookings, venue, 19)

	resp, err := http.Get(server.URL + "/fakepay/" + held.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(page), "Bayar Rp40.000") {
		t.Fatalf("status %d, page:\n%s", resp.StatusCode, page)
	}
}

func TestPayingSendsASignedNotificationAndReturnsThePlayer(t *testing.T) {
	server, bookings, venue := newProvider(t)
	held := holdAt(t, bookings, venue, 19)

	resp := submit(t, server, held.Reference, "pay")
	if resp.StatusCode != http.StatusSeeOther ||
		resp.Header.Get("Location") != "http://web.test/booking/"+held.Reference {
		t.Fatalf("status %d, location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	view, _ := bookings.ByReference(context.Background(), held.Reference)
	if view.Status != store.BookingStatusConfirmed {
		t.Fatalf("booking is %s after paying, want confirmed", view.Status)
	}
}

func TestCancellingThePaymentReleasesTheSlot(t *testing.T) {
	server, bookings, venue := newProvider(t)
	held := holdAt(t, bookings, venue, 19)

	submit(t, server, held.Reference, "cancel")
	view, _ := bookings.ByReference(context.Background(), held.Reference)
	if view.Status != store.BookingStatusCancelled {
		t.Fatalf("booking is %s after cancelling, want cancelled", view.Status)
	}
	holdAt(t, bookings, venue, 19) // the slot is free again
}
