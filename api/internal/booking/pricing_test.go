package booking_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
)

var priceList = []booking.PriceRule{
	{Sport: "badminton", StartsMinute: 7 * 60, EndsMinute: 17 * 60, HourlyPriceIDR: 60000},
	{Sport: "badminton", StartsMinute: 17 * 60, EndsMinute: 23 * 60, HourlyPriceIDR: 80000},
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 11, 20, hour, minute, 0, 0, time.UTC)
}

func TestPriceChargesEachHalfHourAtItsOwnRate(t *testing.T) {
	cases := []struct {
		name       string
		start, end time.Time
		want       int32
	}{
		{"one daytime hour", at(9, 0), at(10, 0), 60000},
		{"one evening hour", at(19, 0), at(20, 0), 80000},
		{"two hours across 17.00", at(16, 0), at(18, 0), 60000 + 80000},
		{"half hour before the change", at(16, 30), at(17, 30), 30000 + 40000},
	}
	for _, c := range cases {
		got, err := booking.Price(priceList, "badminton", c.start, c.end)
		if err != nil || got != c.want {
			t.Errorf("%s: got %d, %v; want %d", c.name, got, err, c.want)
		}
	}
}

func TestPriceRefusesTimeOutsideTheList(t *testing.T) {
	if _, err := booking.Price(priceList, "badminton", at(22, 0), at(24, 0)); !errors.Is(err,
		booking.ErrOutsideOpeningHours) {
		t.Fatalf("got %v, want ErrOutsideOpeningHours", err)
	}
	if _, err := booking.Price(priceList, "futsal", at(9, 0), at(10, 0)); !errors.Is(err,
		booking.ErrOutsideOpeningHours) {
		t.Fatalf("a sport with no rules: got %v, want ErrOutsideOpeningHours", err)
	}
}

func TestDepositIsHalfRoundedUpToAThousand(t *testing.T) {
	for price, want := range map[int32]int32{80000: 40000, 70000: 35000, 75000: 38000, 0: 0} {
		if got := booking.Deposit(price); got != want {
			t.Errorf("Deposit(%d) = %d, want %d", price, got, want)
		}
	}
}

func TestNormalizePhoneAcceptsTheUsualIndonesianForms(t *testing.T) {
	for _, raw := range []string{"081234567890", "6281234567890", "+6281234567890",
		"0812-3456-7890", " 0812 3456 7890 "} {
		got, err := booking.NormalizePhone(raw)
		if err != nil || got != "+6281234567890" {
			t.Errorf("NormalizePhone(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"021555123", "08123", "+6591234567", "nomor saya"} {
		if _, err := booking.NormalizePhone(raw); !errors.Is(err, booking.ErrInvalidRequest) {
			t.Errorf("NormalizePhone(%q): got %v, want ErrInvalidRequest", raw, err)
		}
	}
}
