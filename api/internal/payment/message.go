package payment

import (
	"fmt"
	"strings"
	"time"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
)

type message struct {
	recipient, subject, body string
}

// Messages are written in Indonesian for the people who receive them. Players get them on
// WhatsApp in a real deployment; here the outbox delivers them to a mail catcher or the log.

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

func confirmationMessage(v booking.View) message {
	return message{
		recipient: "whatsapp:" + v.CustomerPhone,
		subject:   "Booking terkonfirmasi " + v.Reference,
		body: fmt.Sprintf("Halo %s, booking %s pada %s sudah terkonfirmasi. DP %s diterima; "+
			"sisa %s dibayar di kasir. Kode booking: %s.",
			v.CustomerName, v.CourtName, slotText(v.Period.Lower.Time, v.Period.Upper.Time),
			Rupiah(v.DepositIdr), Rupiah(v.PriceIdr-v.DepositIdr), v.Reference),
	}
}

func refundAlert(ownerEmail string, v booking.View) message {
	return message{
		recipient: ownerEmail,
		subject:   "DP perlu dikembalikan: " + v.Reference,
		body: fmt.Sprintf("DP %s dari %s (%s) masuk untuk %s pada %s, tetapi slot itu sudah "+
			"tidak tersedia (status booking: %s). Kembalikan DP ke pemain.",
			Rupiah(v.DepositIdr), v.CustomerName, v.CustomerPhone, v.CourtName,
			slotText(v.Period.Lower.Time, v.Period.Upper.Time), v.Status),
	}
}
