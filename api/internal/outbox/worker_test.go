package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/dbtest"
	"github.com/jeevallucas/p10-court-booking/api/internal/outbox"
)

// recordingSender keeps every message it was asked to send, and can be told to fail.
type recordingSender struct {
	mu   sync.Mutex
	sent []outbox.Message
	fail bool
}

func (s *recordingSender) Send(_ context.Context, m outbox.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("mail server unreachable")
	}
	s.sent = append(s.sent, m)
	return nil
}

func queue(t *testing.T, pool *pgxpool.Pool, count int) {
	t.Helper()
	for i := range count {
		if _, err := pool.Exec(context.Background(),
			"insert into outbox_message (recipient, subject, body) values ($1, 'Subjek', 'Isi')",
			"whatsapp:+62812000000"+string(rune('0'+i))); err != nil {
			t.Fatal(err)
		}
	}
}

func worker(pool *pgxpool.Pool, sender outbox.Sender) *outbox.Worker {
	return &outbox.Worker{Pool: pool, Sender: sender, BatchSize: 10,
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}

func TestQueuedMessagesAreSentOnce(t *testing.T) {
	pool := dbtest.New(t)
	queue(t, pool, 3)
	sender := &recordingSender{}
	w := worker(pool, sender)

	if sent, err := w.RunOnce(context.Background()); err != nil || sent != 3 {
		t.Fatalf("first run sent %d, %v; want 3", sent, err)
	}
	if sent, err := w.RunOnce(context.Background()); err != nil || sent != 0 {
		t.Fatalf("second run sent %d, %v; want 0", sent, err)
	}
	if len(sender.sent) != 3 {
		t.Fatalf("sender received %d messages, want 3", len(sender.sent))
	}
}

func TestAFailedMessageIsRetriedLaterNotLost(t *testing.T) {
	pool := dbtest.New(t)
	queue(t, pool, 1)
	sender := &recordingSender{fail: true}
	w := worker(pool, sender)
	ctx := context.Background()

	if sent, err := w.RunOnce(ctx); err != nil || sent != 0 {
		t.Fatalf("failing run sent %d, %v", sent, err)
	}
	var attempts int
	var lastError string
	if err := pool.QueryRow(ctx, "select attempts, last_error from outbox_message").Scan(&attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastError != "mail server unreachable" {
		t.Fatalf("attempts %d, last error %q", attempts, lastError)
	}

	// Not due yet: the retry waits 30 seconds.
	sender.fail = false
	if sent, _ := w.RunOnce(ctx); sent != 0 {
		t.Fatal("a failed message was retried before its delay")
	}
	if _, err := pool.Exec(ctx, "update outbox_message set next_attempt_at = now()"); err != nil {
		t.Fatal(err)
	}
	if sent, err := w.RunOnce(ctx); err != nil || sent != 1 {
		t.Fatalf("retry sent %d, %v; want 1", sent, err)
	}
}

func TestTwoWorkersNeverSendTheSameMessage(t *testing.T) {
	pool := dbtest.New(t)
	queue(t, pool, 8)
	sender := &recordingSender{}
	first, second := worker(pool, sender), worker(pool, sender)
	first.BatchSize, second.BatchSize = 3, 3

	var wg sync.WaitGroup
	for _, w := range []*outbox.Worker{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				sent, err := w.RunOnce(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if sent == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	seen := map[string]int{}
	for _, m := range sender.sent {
		seen[m.Recipient]++
	}
	if len(sender.sent) != 8 || len(seen) != 8 {
		t.Fatalf("sent %d messages to %d recipients, want 8 each exactly once: %v",
			len(sender.sent), len(seen), seen)
	}
}
