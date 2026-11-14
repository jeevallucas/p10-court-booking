// Package ratelimit keeps one token bucket per client key, in memory. One API instance runs
// the demo, so a shared store would add a dependency without a need.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter allows a steady rate per key with a burst, and forgets keys idle for an hour.
type Limiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	clients   map[string]*client
	lastPrune time.Time
	now       func() time.Time
}

type client struct {
	bucket   *rate.Limiter
	lastSeen time.Time
}

const idleAfter = time.Hour

// New allows perMinute requests per key on average and up to burst at once.
func New(perMinute, burst int) *Limiter {
	return &Limiter{
		limit:   rate.Limit(float64(perMinute) / 60),
		burst:   burst,
		clients: map[string]*client{},
		now:     time.Now,
	}
}

// Allow reports whether key may make a request now, and if not, how long until it may.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastPrune) > idleAfter {
		for k, c := range l.clients {
			if now.Sub(c.lastSeen) > idleAfter {
				delete(l.clients, k)
			}
		}
		l.lastPrune = now
	}
	c, ok := l.clients[key]
	if !ok {
		c = &client{bucket: rate.NewLimiter(l.limit, l.burst)}
		l.clients[key] = c
	}
	c.lastSeen = now
	reservation := c.bucket.ReserveN(now, 1)
	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)
		return false, delay
	}
	return true, 0
}
