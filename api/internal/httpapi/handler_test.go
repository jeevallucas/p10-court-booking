package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/httpapi"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/ratelimit"
)

const (
	ownerEmail    = "pemilik@arenakotabaru.test"
	ownerPassword = "lapangan-enam-2026"
	testDay       = "2026-11-20"
)

var wib = func() *time.Location {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err)
	}
	return location
}()

func at(hour, minute int) time.Time {
	return time.Date(2026, 11, 20, hour, minute, 0, 0, wib)
}

type testAPI struct {
	url    string
	client *http.Client
	venue  dbtest.Venue
}

// newAPI starts the real handler on a fresh database whose clock reads 20 November, 10.00 WIB.
func newAPI(t *testing.T, holdLimiter *ratelimit.Limiter) testAPI {
	t.Helper()
	pool := dbtest.New(t)
	venue := dbtest.NewVenue(t, pool)
	hash, err := owner.HashPassword(ownerPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `insert into owner_account
		(venue_id, email, password_hash, display_name) values ($1, $2, $3, 'Bu Ratna')`,
		venue.ID, ownerEmail, hash); err != nil {
		t.Fatal(err)
	}
	if holdLimiter == nil {
		holdLimiter = ratelimit.New(1000, 1000)
	}
	clock := at(10, 0)
	handler, err := httpapi.NewHandler(httpapi.Config{
		Bookings:      &booking.Service{Pool: pool, Now: func() time.Time { return clock }},
		Sessions:      &owner.Sessions{Pool: pool, Lifetime: time.Hour},
		Logger:        slog.New(slog.NewJSONHandler(io.Discard, nil)),
		SecureCookies: false,
		HoldLimiter:   holdLimiter,
		LoginLimiter:  ratelimit.New(1000, 1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	return testAPI{url: server.URL, client: &http.Client{Jar: jar}, venue: venue}
}

func (a testAPI) do(t *testing.T, method, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, a.url+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	decoded := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s %s: response is not JSON: %s", method, path, raw)
		}
	}
	return resp, decoded
}

func expect(t *testing.T, resp *http.Response, body map[string]any, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d; body %v", resp.StatusCode, status, body)
	}
	if code != "" && body["code"] != code {
		t.Fatalf("problem code %v, want %s; body %v", body["code"], code, body)
	}
}

func holdBody(court uuid.UUID, start time.Time, hours int) map[string]any {
	return map[string]any{
		"courtId": court, "start": start.Format(time.RFC3339), "hours": hours,
		"customerName": "Rizky Pratama", "customerPhone": "081234567890",
	}
}

func court(id [16]byte) uuid.UUID { return uuid.UUID(id) }

func TestAvailabilityListsTheDayForOneSport(t *testing.T) {
	api := newAPI(t, nil)
	resp, body := api.do(t, http.MethodGet, "/v1/availability?date="+testDay+"&sport=futsal", nil)
	expect(t, resp, body, http.StatusOK, "")
	if courts := body["courts"].([]any); len(courts) != 2 {
		t.Fatalf("%d futsal courts, want 2", len(courts))
	}
	if slots := body["slots"].([]any); len(slots) != 2*16 {
		t.Fatalf("%d slots, want 32", len(slots))
	}

	resp, body = api.do(t, http.MethodGet, "/v1/availability?date="+testDay+"&sport=tenis", nil)
	expect(t, resp, body, http.StatusUnprocessableEntity, "invalid_request")
}

func TestAPlayerHoldsASlotAndASecondPlayerIsTurnedAway(t *testing.T) {
	api := newAPI(t, nil)
	courtID := court(api.venue.Badminton[0].Bytes)

	resp, held := api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(19, 0), 1))
	expect(t, resp, held, http.StatusCreated, "")
	if held["status"] != "held" || held["priceIdr"] != float64(80000) || held["depositIdr"] != float64(40000) {
		t.Fatalf("hold %v", held)
	}

	resp, body := api.do(t, http.MethodGet, "/v1/bookings/"+held["reference"].(string), nil)
	expect(t, resp, body, http.StatusOK, "")
	if _, leaked := body["customerPhone"]; leaked {
		t.Fatal("the public booking view exposes the phone number")
	}

	resp, body = api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(19, 0), 1))
	expect(t, resp, body, http.StatusConflict, "slot_taken")
}

func TestHoldRequestsThatBreakTheContractOrTheRulesAreRefused(t *testing.T) {
	api := newAPI(t, nil)
	courtID := court(api.venue.Badminton[0].Bytes)

	badPhone := holdBody(courtID, at(19, 0), 1)
	badPhone["customerPhone"] = "021555123"
	resp, body := api.do(t, http.MethodPost, "/v1/holds", badPhone)
	expect(t, resp, body, http.StatusUnprocessableEntity, "invalid_request")

	resp, body = api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(19, 0), 3))
	expect(t, resp, body, http.StatusUnprocessableEntity, "invalid_request")

	resp, body = api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(9, 0), 1))
	expect(t, resp, body, http.StatusUnprocessableEntity, "in_the_past")

	resp, body = api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(22, 0), 2))
	expect(t, resp, body, http.StatusUnprocessableEntity, "outside_opening_hours")

	resp, body = api.do(t, http.MethodGet, "/v1/bookings/ZZZZZZZZ", nil)
	expect(t, resp, body, http.StatusNotFound, "not_found")
}

func TestTooManyHoldsFromOneAddressAreRateLimited(t *testing.T) {
	api := newAPI(t, ratelimit.New(10, 2))
	courtID := court(api.venue.Badminton[0].Bytes)
	for hour := 18; hour < 20; hour++ {
		resp, body := api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(hour, 0), 1))
		expect(t, resp, body, http.StatusCreated, "")
	}
	resp, body := api.do(t, http.MethodPost, "/v1/holds", holdBody(courtID, at(21, 0), 1))
	expect(t, resp, body, http.StatusTooManyRequests, "rate_limited")
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("no Retry-After header")
	}
}

func TestEveryOwnerEndpointNeedsASession(t *testing.T) {
	api := newAPI(t, nil)
	someID := "/v1/owner/bookings/" + uuid.NewString()
	window := map[string]any{"courtId": uuid.New(), "start": at(19, 0).Format(time.RFC3339),
		"end": at(20, 0).Format(time.RFC3339), "customerName": "Tanpa Sesi"}
	requests := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/v1/owner/bookings?date=" + testDay, nil},
		{http.MethodPost, "/v1/owner/walk-ins", window},
		{http.MethodPatch, someID, window},
		{http.MethodPost, someID + "/status", map[string]any{"status": "cancelled"}},
		{http.MethodDelete, "/v1/owner/session", nil},
	}
	for _, r := range requests {
		resp, body := api.do(t, r.method, r.path, r.body)
		if resp.StatusCode != http.StatusUnauthorized || body["code"] != "unauthorized" {
			t.Errorf("%s %s without a session: %d %v", r.method, r.path, resp.StatusCode, body)
		}
	}

	resp, body := api.do(t, http.MethodPost, "/v1/owner/session",
		map[string]any{"email": ownerEmail, "password": "salah"})
	expect(t, resp, body, http.StatusUnauthorized, "unauthorized")
}

func TestTheOwnerRunsTheDay(t *testing.T) {
	api := newAPI(t, nil)
	first := court(api.venue.Badminton[0].Bytes)
	second := court(api.venue.Badminton[1].Bytes)

	resp, body := api.do(t, http.MethodPost, "/v1/owner/session",
		map[string]any{"email": ownerEmail, "password": ownerPassword})
	expect(t, resp, body, http.StatusNoContent, "")
	setCookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "HttpOnly") || !strings.Contains(setCookie, "SameSite=Lax") {
		t.Fatalf("session cookie %q is not HttpOnly and SameSite=Lax", setCookie)
	}

	resp, held := api.do(t, http.MethodPost, "/v1/holds", holdBody(first, at(19, 0), 1))
	expect(t, resp, held, http.StatusCreated, "")

	resp, walkIn := api.do(t, http.MethodPost, "/v1/owner/walk-ins", map[string]any{
		"courtId": second, "start": at(19, 0).Format(time.RFC3339),
		"end": at(20, 30).Format(time.RFC3339), "customerName": "Pak Hendra",
	})
	expect(t, resp, walkIn, http.StatusCreated, "")
	if walkIn["status"] != "walk_in" {
		t.Fatalf("walk-in %v", walkIn)
	}

	resp, day := api.do(t, http.MethodGet, "/v1/owner/bookings?date="+testDay, nil)
	expect(t, resp, day, http.StatusOK, "")
	bookings := day["bookings"].([]any)
	if len(bookings) != 2 || bookings[0].(map[string]any)["customerPhone"] != "+6281234567890" {
		t.Fatalf("owner day %v", day)
	}

	walkInPath := "/v1/owner/bookings/" + walkIn["id"].(string)
	resp, body = api.do(t, http.MethodPatch, walkInPath, map[string]any{
		"courtId": first, "start": at(18, 30).Format(time.RFC3339), "end": at(19, 30).Format(time.RFC3339),
	})
	expect(t, resp, body, http.StatusConflict, "slot_taken")

	resp, body = api.do(t, http.MethodPost, walkInPath+"/status", map[string]any{"status": "no_show"})
	expect(t, resp, body, http.StatusConflict, "invalid_transition")

	resp, body = api.do(t, http.MethodPost, walkInPath+"/status", map[string]any{"status": "cancelled"})
	expect(t, resp, body, http.StatusOK, "")

	resp, body = api.do(t, http.MethodDelete, "/v1/owner/session", nil)
	expect(t, resp, body, http.StatusNoContent, "")
	resp, body = api.do(t, http.MethodGet, "/v1/owner/bookings?date="+testDay, nil)
	expect(t, resp, body, http.StatusUnauthorized, "unauthorized")
}

func TestEveryResponseCarriesARequestID(t *testing.T) {
	api := newAPI(t, nil)
	resp, _ := api.do(t, http.MethodGet, "/v1/availability?date="+testDay+"&sport=badminton", nil)
	if len(resp.Header.Get("X-Request-ID")) != 16 {
		t.Fatalf("X-Request-ID %q", resp.Header.Get("X-Request-ID"))
	}
}
