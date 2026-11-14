// Package notify writes the messages players and the owner receive and queues them in the
// outbox inside the caller's transaction, so a change and its message commit together.
// Messages are in Indonesian for the people who read them. Players get them on WhatsApp in a
// real deployment; here the outbox delivers them to a mail catcher or the log.
package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

// Booking holds what a message says about a booking.
type Booking struct {
	Reference     string
	CourtName     string
	CustomerName  string
	CustomerPhone string // empty for a walk-in who left no number
	Start, End    time.Time
	PriceIDR      int32
	DepositIDR    int32
	Status        string
}

// Message is one outbox entry.
type Message struct {
	Recipient, Subject, Body string
}

var (
	weekdays = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months   = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli",
		"Agustus", "September", "Oktober", "November", "Desember"}
	jakarta = mustLocation("Asia/Jakarta")
)

func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

// Queue adds the message to the outbox; a message without a recipient is skipped.
func Queue(ctx context.Context, q *store.Queries, m Message) error {
	if m.Recipient == "" {
		return nil
	}
	if err := q.EnqueueMessage(ctx, store.EnqueueMessageParams{
		Recipient: m.Recipient, Subject: m.Subject, Body: m.Body,
	}); err != nil {
		return fmt.Errorf("queue message: %w", err)
	}
	return nil
}

func player(b Booking) string {
	if b.CustomerPhone == "" {
		return ""
	}
	return "whatsapp:" + b.CustomerPhone
}

// Confirmation tells the player the booking is confirmed and what is left to pay.
func Confirmation(b Booking) Message {
	return Message{
		Recipient: player(b),
		Subject:   "Booking terkonfirmasi " + b.Reference,
		Body: fmt.Sprintf("Halo %s, booking %s pada %s sudah terkonfirmasi. DP %s diterima; "+
			"sisa %s dibayar di kasir. Kode booking: %s.",
			b.CustomerName, b.CourtName, slotText(b.Start, b.End),
			Rupiah(b.DepositIDR), Rupiah(b.PriceIDR-b.DepositIDR), b.Reference),
	}
}

// Moved tells the player the owner moved the booking, from where to where.
func Moved(b Booking, fromCourt string, fromStart, fromEnd time.Time) Message {
	return Message{
		Recipient: player(b),
		Subject:   "Booking dipindah " + b.Reference,
		Body: fmt.Sprintf("Halo %s, pengelola memindahkan booking kamu dari %s, %s ke %s, %s. "+
			"Kode booking tetap %s. Kalau jam baru tidak cocok, hubungi Arena Kotabaru.",
			b.CustomerName, fromCourt, slotText(fromStart, fromEnd), b.CourtName,
			slotText(b.Start, b.End), b.Reference),
	}
}

// Cancelled tells the player the owner cancelled the booking, and whether a deposit comes back.
func Cancelled(b Booking, depositPaid bool) Message {
	refund := ""
	if depositPaid {
		refund = fmt.Sprintf(" DP %s akan dikembalikan oleh pengelola.", Rupiah(b.DepositIDR))
	}
	return Message{
		Recipient: player(b),
		Subject:   "Booking dibatalkan " + b.Reference,
		Body: fmt.Sprintf("Halo %s, booking %s pada %s dibatalkan oleh pengelola.%s",
			b.CustomerName, b.CourtName, slotText(b.Start, b.End), refund),
	}
}

// RefundAlert asks the owner to return a deposit that has no slot to go with it.
func RefundAlert(ownerEmail string, b Booking) Message {
	return Message{
		Recipient: ownerEmail,
		Subject:   "DP perlu dikembalikan: " + b.Reference,
		Body: fmt.Sprintf("DP %s dari %s (%s) untuk %s pada %s tidak punya slot lagi "+
			"(status booking: %s). Kembalikan DP ke pemain.",
			Rupiah(b.DepositIDR), b.CustomerName, b.CustomerPhone, b.CourtName,
			slotText(b.Start, b.End), b.Status),
	}
}

// slotText renders "Jumat, 20 November 2026, 19.00–20.00 WIB".
func slotText(start, end time.Time) string {
	start, end = start.In(jakarta), end.In(jakarta)
	return fmt.Sprintf("%s, %d %s %d, %s–%s WIB", weekdays[start.Weekday()], start.Day(),
		months[start.Month()-1], start.Year(), start.Format("15.04"), end.Format("15.04"))
}

// Rupiah renders 40000 as "Rp40.000".
func Rupiah(amount int32) string {
	digits := fmt.Sprint(amount)
	var groups []string
	for len(digits) > 3 {
		groups = append([]string{digits[len(digits)-3:]}, groups...)
		digits = digits[:len(digits)-3]
	}
	groups = append([]string{digits}, groups...)
	return "Rp" + strings.Join(groups, ".")
}
