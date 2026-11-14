package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	middleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/jeevallucas/p10-court-booking/api/internal/booking"
	"github.com/jeevallucas/p10-court-booking/api/internal/owner"
	"github.com/jeevallucas/p10-court-booking/api/internal/payment"
	"github.com/jeevallucas/p10-court-booking/api/internal/ratelimit"
)

// Config wires the HTTP layer to the booking rules and owner sessions.
type Config struct {
	Bookings *booking.Service
	Sessions *owner.Sessions
	Payments *payment.Processor
	Logger   *slog.Logger
	// PaymentPageURL is the base of the deposit page, ending in "/"; the reference is appended.
	PaymentPageURL string
	SecureCookies  bool
	// TrustedProxyHops is how many proxies in front of the API append to X-Forwarded-For;
	// the client address is the entry that many places from the right. 0 uses the TCP peer.
	TrustedProxyHops int
	HoldLimiter      *ratelimit.Limiter
	LoginLimiter     *ratelimit.Limiter
}

const maxBodyBytes = 64 << 10

// NewHandler returns the API: request IDs and logs, a body size limit, validation against
// openapi.yaml, rate limits, owner sessions, then the handlers.
func NewHandler(cfg Config) (http.Handler, error) {
	spec, err := GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI spec: %w", err)
	}
	spec.Servers = nil

	srv := &server{
		bookings:       cfg.Bookings,
		sessions:       cfg.Sessions,
		payments:       cfg.Payments,
		secureCookies:  cfg.SecureCookies,
		paymentPageURL: cfg.PaymentPageURL,
	}
	// Strict middleware receives the generated handler name, e.g. "PlaceHold".
	limits := map[string]*ratelimit.Limiter{"PlaceHold": cfg.HoldLimiter, "LogIn": cfg.LoginLimiter}
	strict := NewStrictHandlerWithOptions(srv,
		[]StrictMiddlewareFunc{
			requireOwner(cfg.Sessions, ownerOperations(spec)),
			limitRate(limits, cfg.TrustedProxyHops),
		},
		StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				writeProblem(w, http.StatusUnprocessableEntity, problem(InvalidRequest, err.Error()))
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				loggerFrom(r.Context(), cfg.Logger).Error("request failed", "error", err)
				writeProblem(w, http.StatusInternalServerError,
					problem(InvalidRequest, "the server could not complete the request; try again"))
			},
		})

	routes := HandlerWithOptions(strict, StdHTTPServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, http.StatusUnprocessableEntity, problem(InvalidRequest, err.Error()))
		},
	})
	validated := middleware.OapiRequestValidatorWithOptions(spec, &middleware.Options{
		SilenceServersWarning: true,
		Options: openapi3filter.Options{
			// Sessions are checked by requireOwner, which can tell a missing cookie from an
			// expired one.
			AuthenticationFunc: func(context.Context, *openapi3filter.AuthenticationInput) error { return nil },
		},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, _ *http.Request,
			opts middleware.ErrorHandlerOpts) {
			if opts.StatusCode == http.StatusNotFound {
				writeProblem(w, http.StatusNotFound, problem(NotFound, "no such endpoint"))
				return
			}
			writeProblem(w, http.StatusUnprocessableEntity, problem(InvalidRequest, err.Error()))
		},
	})(routes)

	return withRequestLog(cfg.Logger, cfg.TrustedProxyHops, http.MaxBytesHandler(validated, maxBodyBytes)), nil
}

type contextKey int

const (
	loggerKey contextKey = iota
	tokenKey
)

func loggerFrom(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return fallback
}

func sessionToken(ctx context.Context) string {
	token, _ := ctx.Value(tokenKey).(string)
	return token
}

// ownerOperations reads which operations need a session from the spec's security
// requirements, so a new owner endpoint is protected as soon as the spec says so. Keys are the
// generated handler names (operationId with a capital first letter).
func ownerOperations(spec *openapi3.T) map[string]bool {
	protected := map[string]bool{}
	for _, item := range spec.Paths.Map() {
		for _, operation := range item.Operations() {
			if operation.Security != nil && len(*operation.Security) > 0 {
				name := operation.OperationID
				protected[strings.ToUpper(name[:1])+name[1:]] = true
			}
		}
	}
	return protected
}

func requireOwner(sessions *owner.Sessions, protected map[string]bool) StrictMiddlewareFunc {
	return func(next StrictHandlerFunc, operationID string) StrictHandlerFunc {
		if !protected[operationID] {
			return next
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
			cookie, err := r.Cookie(SessionCookie)
			if err != nil {
				writeProblem(w, http.StatusUnauthorized, problem(Unauthorized, "log in as the owner first"))
				return nil, nil
			}
			_, err = sessions.Lookup(ctx, cookie.Value)
			if errors.Is(err, owner.ErrNoSession) {
				writeProblem(w, http.StatusUnauthorized,
					problem(Unauthorized, "the session has ended; log in again"))
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			ctx = context.WithValue(ctx, tokenKey, cookie.Value)
			return next(ctx, w, r, req)
		}
	}
}

func limitRate(limits map[string]*ratelimit.Limiter, trustedHops int) StrictMiddlewareFunc {
	return func(next StrictHandlerFunc, operationID string) StrictHandlerFunc {
		limiter := limits[operationID]
		if limiter == nil {
			return next
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
			if ok, wait := limiter.Allow(clientIP(r, trustedHops)); !ok {
				w.Header().Set("Retry-After", fmt.Sprint(int(math.Ceil(wait.Seconds()))))
				writeProblem(w, http.StatusTooManyRequests,
					problem(RateLimited, "too many requests from this address; wait and try again"))
				return nil, nil
			}
			return next(ctx, w, r, req)
		}
	}
}

// clientIP takes the address the last trusted proxy saw, so a client cannot dodge the limit by
// sending its own X-Forwarded-For.
func clientIP(r *http.Request, trustedHops int) string {
	if trustedHops > 0 {
		var hops []string
		for _, header := range r.Header.Values("X-Forwarded-For") {
			for _, part := range strings.Split(header, ",") {
				hops = append(hops, strings.TrimSpace(part))
			}
		}
		if len(hops) >= trustedHops {
			return hops[len(hops)-trustedHops]
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeProblem(w http.ResponseWriter, status int, p ProblemJSONResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// withRequestLog tags each request with an ID (the caller's, if it sent a sane one), returns it
// in X-Request-ID, and logs one JSON line per request. The line includes the client address the
// rate limiter uses, which shows whether TRUSTED_PROXY_HOPS matches the proxies in front.
func withRequestLog(logger *slog.Logger, trustedHops int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			raw := make([]byte, 8)
			_, _ = rand.Read(raw)
			id = hex.EncodeToString(raw)
		}
		w.Header().Set("X-Request-ID", id)
		requestLogger := logger.With("request_id", id)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), loggerKey, requestLogger)))
		requestLogger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"client", clientIP(r, trustedHops),
			"duration_ms", time.Since(started).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}
