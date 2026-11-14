package httpapi

import (
	"net/http/httptest"
	"testing"
)

// The rate limit is keyed by this address, and on the demo host TRUSTED_PROXY_HOPS=2 because
// X-Forwarded-For arrives as "<visitor>, 127.0.0.1" (docs/decisions.md).
func TestTheClientAddressComesFromTheTrustedProxyHop(t *testing.T) {
	cases := []struct {
		name    string
		hops    int
		headers []string
		want    string
	}{
		{"no proxy trusted", 0, []string{"203.0.113.9"}, "192.0.2.1"},
		{"one proxy", 1, []string{"198.51.100.7"}, "198.51.100.7"},
		{"Vercel: visitor then the local bridge", 2, []string{"198.51.100.7, 127.0.0.1"}, "198.51.100.7"},
		{"a forged first entry is ignored", 2, []string{"203.0.113.9, 198.51.100.7, 127.0.0.1"}, "198.51.100.7"},
		{"split across two headers", 2, []string{"198.51.100.7", "127.0.0.1"}, "198.51.100.7"},
		{"fewer entries than hops", 2, []string{"127.0.0.1"}, "192.0.2.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/availability", nil)
			r.RemoteAddr = "192.0.2.1:41234"
			for _, h := range c.headers {
				r.Header.Add("X-Forwarded-For", h)
			}
			if got := clientIP(r, c.hops); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}
