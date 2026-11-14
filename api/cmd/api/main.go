// Command api serves the booking API, applies migrations on start, and runs the background
// sweeps that expire holds and old sessions.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // the venue's zone must load even in a minimal container image

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/fakepay"
	"github.com/jeevallucas/p10-court-booking/api/internal/httpapi"
	"github.com/jeevallucas/p10-court-booking/api/internal/migrations"
	"github.com/jeevallucas/p10-court-booking/api/internal/outbox"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/payment"
	"github.com/jeevallucas/p10-court-booking/api/internal/ratelimit"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is not set; copy .env.example to .env or set it in the host's settings")
	}
	port := envOr("PORT", "8080")
	secureCookies, err := strconv.ParseBool(envOr("COOKIE_SECURE", "true"))
	if err != nil {
		return fmt.Errorf("COOKIE_SECURE must be true or false: %w", err)
	}
	trustedHops, err := strconv.Atoi(envOr("TRUSTED_PROXY_HOPS", "0"))
	if err != nil || trustedHops < 0 {
		return fmt.Errorf("TRUSTED_PROXY_HOPS must be a whole number of proxies, got %q",
			os.Getenv("TRUSTED_PROXY_HOPS"))
	}
	serverKey := os.Getenv("PAYMENT_SERVER_KEY")
	if serverKey == "" {
		return errors.New("PAYMENT_SERVER_KEY is not set; it signs and checks payment notifications")
	}
	// The browser reaches the API and the fake payment page through the web app's proxy.
	webURL := envOr("PUBLIC_WEB_URL", "http://localhost:3000")
	notificationURL := envOr("NOTIFICATION_URL", "http://127.0.0.1:"+port+"/v1/payments/notifications")
	ownerEmail := envOr("OWNER_ALERT_EMAIL", "pemilik@arenakotabaru.test")
	var sender outbox.Sender = outbox.LogSender{Logger: logger}
	if smtpAddr := os.Getenv("SMTP_ADDR"); smtpAddr != "" {
		sender = outbox.SMTPSender{Addr: smtpAddr, From: "booking@arenakotabaru.test"}
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to the database: %w", err)
	}
	defer pool.Close()
	migrationDB := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(ctx, migrationDB); err != nil {
		return err
	}
	if err := migrationDB.Close(); err != nil {
		return fmt.Errorf("close migration connection: %w", err)
	}

	bookings := &booking.Service{Pool: pool, Now: time.Now}
	sessions := &owner.Sessions{Pool: pool, Lifetime: 12 * time.Hour}
	api, err := httpapi.NewHandler(httpapi.Config{
		Bookings:         bookings,
		Sessions:         sessions,
		Payments:         &payment.Processor{Pool: pool, ServerKey: serverKey, OwnerEmail: ownerEmail},
		PaymentPageURL:   webURL + "/fakepay/",
		Logger:           logger,
		SecureCookies:    secureCookies,
		TrustedProxyHops: trustedHops,
		HoldLimiter:      ratelimit.New(10, 5),
		LoginLimiter:     ratelimit.New(5, 5),
	})
	if err != nil {
		return err
	}
	handler := http.NewServeMux()
	handler.Handle("/", api)
	// The host checks this before routing traffic to a new deploy; it fails without the database.
	handler.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler.Handle("/fakepay/", fakepay.Handler(fakepay.Config{
		Bookings:        bookings,
		ServerKey:       serverKey,
		NotificationURL: notificationURL,
		ReturnURL:       webURL + "/booking/",
		Client:          &http.Client{Timeout: 10 * time.Second},
		Logger:          logger,
	}))

	worker := &outbox.Worker{Pool: pool, Sender: sender, BatchSize: 20, Logger: logger}
	go every(ctx, 2*time.Second, func() {
		if _, err := worker.RunOnce(ctx); err != nil {
			logger.Error("send outbox messages", "error", err)
		}
	})
	go every(ctx, 30*time.Second, func() {
		if expired, err := bookings.ExpireHolds(ctx); err != nil {
			logger.Error("expire holds", "error", err)
		} else if expired > 0 {
			logger.Info("expired holds", "count", expired)
		}
	})
	go every(ctx, time.Hour, func() {
		if _, err := sessions.DeleteExpired(ctx); err != nil {
			logger.Error("delete expired sessions", "error", err)
		}
	})

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "port", port)
		serveErr <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	return nil
}

func every(ctx context.Context, interval time.Duration, task func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			task()
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
