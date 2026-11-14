package ratelimit

import (
	"testing"
	"time"
)

func TestABurstIsAllowedThenRefusedWithAWait(t *testing.T) {
	limiter := New(10, 3)
	now := time.Date(2026, 11, 20, 19, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for i := range 3 {
		if ok, _ := limiter.Allow("203.0.113.7"); !ok {
			t.Fatalf("request %d of the burst refused", i+1)
		}
	}
	ok, wait := limiter.Allow("203.0.113.7")
	if ok || wait <= 0 || wait > 6*time.Second {
		t.Fatalf("fourth request: allowed=%v wait=%v; want refused with about 6s", ok, wait)
	}
	if ok, _ := limiter.Allow("198.51.100.4"); !ok {
		t.Fatal("another client was refused")
	}

	now = now.Add(6 * time.Second)
	if ok, _ := limiter.Allow("203.0.113.7"); !ok {
		t.Fatal("refused again after waiting for one token")
	}
}

func TestIdleClientsAreForgotten(t *testing.T) {
	limiter := New(10, 1)
	now := time.Date(2026, 11, 20, 19, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	limiter.Allow("203.0.113.7")

	now = now.Add(2 * time.Hour)
	limiter.Allow("198.51.100.4")
	if _, ok := limiter.clients["203.0.113.7"]; ok {
		t.Fatal("a client idle for two hours is still kept")
	}
}
