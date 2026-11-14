// Package outbox delivers messages that were queued in the same transaction as the change they
// report. A message is marked sent only after delivery, so a crash means a resend, never a loss.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

// Message is one queued notification.
type Message struct {
	Recipient, Subject, Body string
}

// Sender delivers a message: to a mail catcher in development, to the log in the demo.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Worker sends due messages in batches.
type Worker struct {
	Pool      *pgxpool.Pool
	Sender    Sender
	BatchSize int32
	Logger    *slog.Logger
}

// maxRetryDelay caps the doubling delay between attempts for a message that keeps failing.
const maxRetryDelay = time.Hour

// RunOnce sends one batch of due messages and reports how many were sent. The batch's rows stay
// locked until the transaction ends, so a second worker skips them instead of sending twice.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	sent := 0
	err := pgx.BeginFunc(ctx, w.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		due, err := q.ClaimDueMessages(ctx, w.BatchSize)
		if err != nil {
			return fmt.Errorf("claim messages: %w", err)
		}
		for _, row := range due {
			sendErr := w.Sender.Send(ctx, Message{Recipient: row.Recipient, Subject: row.Subject, Body: row.Body})
			if sendErr == nil {
				if err := q.MarkMessageSent(ctx, row.ID); err != nil {
					return fmt.Errorf("mark message %d sent: %w", row.ID, err)
				}
				sent++
				continue
			}
			delay := retryDelay(row.Attempts)
			w.Logger.Warn("message not sent; will retry", "message_id", row.ID,
				"attempt", row.Attempts+1, "retry_in", delay.String(), "error", sendErr)
			if err := q.MarkMessageFailed(ctx, store.MarkMessageFailedParams{
				ID:        row.ID,
				LastError: pgtype.Text{String: sendErr.Error(), Valid: true},
				RetryIn:   pgtype.Interval{Microseconds: delay.Microseconds(), Valid: true},
			}); err != nil {
				return fmt.Errorf("record failure of message %d: %w", row.ID, err)
			}
		}
		return nil
	})
	return sent, err
}

func retryDelay(previousAttempts int32) time.Duration {
	delay := 30 * time.Second
	for range previousAttempts {
		delay *= 2
		if delay >= maxRetryDelay {
			return maxRetryDelay
		}
	}
	return delay
}

// LogSender writes messages to the structured log; the demo has no real messaging.
type LogSender struct {
	Logger *slog.Logger
}

// Send logs the message.
func (s LogSender) Send(_ context.Context, m Message) error {
	s.Logger.Info("message", "recipient", m.Recipient, "subject", m.Subject, "body", m.Body)
	return nil
}

// SMTPSender delivers to an SMTP server without authentication, such as Mailpit in development.
// WhatsApp recipients become addresses on the reserved .test domain so they show up there too.
type SMTPSender struct {
	Addr string
	From string
}

// Send delivers the message as a plain-text email.
func (s SMTPSender) Send(ctx context.Context, m Message) error {
	to := m.Recipient
	if phone, ok := strings.CutPrefix(to, "whatsapp:"); ok {
		to = strings.TrimPrefix(phone, "+") + "@whatsapp.test"
	}
	body := "From: " + s.From + "\r\nTo: " + to + "\r\nSubject: " + m.Subject +
		"\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + m.Body + "\r\n"
	if err := s.deliver(ctx, to, []byte(body)); err != nil {
		return fmt.Errorf("send mail to %s via %s: %w", to, s.Addr, err)
	}
	return nil
}

// sendTimeout bounds one delivery. smtp.SendMail has none, and the worker sends inside the
// transaction that claimed the message, so a hung mail server would pin a connection.
const sendTimeout = 10 * time.Second

func (s SMTPSender) deliver(ctx context.Context, to string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return err
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		_ = conn.Close()
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = client.Close() }()
	if err := client.Mail(s.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
