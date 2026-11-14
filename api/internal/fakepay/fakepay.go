// Package fakepay stands in for the payment provider's checkout page. It shows the deposit,
// and when the player pays or cancels it sends the API a notification in Midtrans's format,
// signed with the server key, over HTTP, as the real provider would.
package fakepay

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/payment"
	"github.com/jeevallucas/p10-court-booking/api/internal/store"
)

// Config connects the fake provider to the booking data and the API's notification endpoint.
type Config struct {
	Bookings        *booking.Service
	ServerKey       string
	NotificationURL string
	// ReturnURL is where the player lands after paying; the reference is appended.
	ReturnURL string
	Client    *http.Client
	Logger    *slog.Logger
}

// Handler serves GET and POST /fakepay/{reference}.
func Handler(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /fakepay/{reference}", func(w http.ResponseWriter, r *http.Request) {
		view, err := cfg.Bookings.ByReference(r.Context(), r.PathValue("reference"))
		if errors.Is(err, booking.ErrNotFound) {
			http.Error(w, "Booking tidak ditemukan.", http.StatusNotFound)
			return
		}
		if err != nil {
			cfg.Logger.Error("fake payment page", "error", err)
			http.Error(w, "Halaman pembayaran gagal dimuat. Muat ulang halaman.", http.StatusInternalServerError)
			return
		}
		render(w, view)
	})
	mux.HandleFunc("POST /fakepay/{reference}", func(w http.ResponseWriter, r *http.Request) {
		reference := r.PathValue("reference")
		view, err := cfg.Bookings.ByReference(r.Context(), reference)
		if err != nil {
			http.Error(w, "Booking tidak ditemukan.", http.StatusNotFound)
			return
		}
		status := "settlement"
		if r.FormValue("action") == "cancel" {
			status = "cancel"
		}
		if err := notify(r.Context(), cfg, view, status); err != nil {
			cfg.Logger.Error("fake payment notification", "reference", reference, "error", err)
			http.Error(w, "Notifikasi pembayaran gagal terkirim ke server booking. Coba lagi.",
				http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, cfg.ReturnURL+reference, http.StatusSeeOther)
	})
	return mux
}

// notification is the JSON body Midtrans posts; field names follow its documentation.
type notification struct {
	TransactionTime   string `json:"transaction_time"`
	TransactionStatus string `json:"transaction_status"`
	TransactionID     string `json:"transaction_id"`
	StatusMessage     string `json:"status_message"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
	PaymentType       string `json:"payment_type"`
	OrderID           string `json:"order_id"`
	MerchantID        string `json:"merchant_id"`
	GrossAmount       string `json:"gross_amount"`
	FraudStatus       string `json:"fraud_status"`
	Currency          string `json:"currency"`
}

func notify(ctx context.Context, cfg Config, view booking.View, status string) error {
	statusCode := "200"
	if status != "settlement" {
		statusCode = "202"
	}
	transactionID := make([]byte, 16)
	if _, err := rand.Read(transactionID); err != nil {
		return err
	}
	gross := fmt.Sprintf("%d.00", view.DepositIdr)
	body, err := json.Marshal(notification{
		TransactionTime:   time.Now().In(jakarta).Format("2006-01-02 15:04:05"),
		TransactionStatus: status,
		TransactionID:     hex.EncodeToString(transactionID),
		StatusMessage:     "fake provider notification",
		StatusCode:        statusCode,
		SignatureKey:      payment.Signature(view.Reference, statusCode, gross, cfg.ServerKey),
		PaymentType:       "qris",
		OrderID:           view.Reference,
		MerchantID:        "LOCAL-FAKE",
		GrossAmount:       gross,
		FraudStatus:       "accept",
		Currency:          "IDR",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.NotificationURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("post to %s: %w", cfg.NotificationURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("notification endpoint answered %s", resp.Status)
	}
	return nil
}

var jakarta = func() *time.Location {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err)
	}
	return location
}()

var page = template.Must(template.New("pay").Parse(`<!doctype html>
<html lang="id">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bayar DP {{.Reference}}</title>
<style>
body { font-family: system-ui, sans-serif; background: #f4f6f1; color: #16201b; margin: 0; }
main { max-width: 26rem; margin: 2rem auto; padding: 1.5rem; background: #fff;
  border: 1px solid #c9cfc9; border-radius: 4px; }
h1 { font-size: 1.25rem; margin: 0 0 0.5rem; }
.note { color: #55605a; font-size: 0.875rem; }
.amount { font-size: 2rem; font-weight: 700; margin: 1rem 0; }
button { width: 100%; min-height: 48px; font-size: 1rem; border-radius: 4px; margin-top: 0.75rem;
  cursor: pointer; }
.pay { background: #16201b; color: #fff; border: 0; }
.cancel { background: #fff; color: #16201b; border: 1px solid #16201b; }
button:focus-visible { outline: 3px solid #14684a; outline-offset: 2px; }
</style>
</head>
<body>
<main>
  <h1>Simulasi pembayaran DP</h1>
  <p class="note">Penyedia pembayaran tiruan untuk demo. Tidak ada uang yang berpindah.</p>
  <p>{{.CourtName}}, kode booking {{.Reference}}</p>
  {{if .Payable}}
  <p class="amount">{{.Deposit}}</p>
  <form method="post">
    <button class="pay" name="action" value="pay">Bayar {{.Deposit}}</button>
    <button class="cancel" name="action" value="cancel">Batalkan pembayaran</button>
  </form>
  {{else}}
  <p>Booking ini tidak menunggu pembayaran lagi (status: {{.Status}}).</p>
  {{end}}
</main>
</body>
</html>`))

func render(w http.ResponseWriter, view booking.View) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, map[string]any{
		"Reference": view.Reference,
		"CourtName": view.CourtName,
		"Deposit":   payment.Rupiah(view.DepositIdr),
		"Payable":   view.Status == store.BookingStatusHeld,
		"Status":    view.Status,
	})
}
