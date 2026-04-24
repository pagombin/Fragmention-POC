// Package middleware provides the authentication, request-scoping, rate
// limiting, and security-header middleware applied to every API route.
package middleware

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyLogger
)

// RequestID ensures every request carries an X-Request-ID value, minting one
// when absent. Downstream handlers retrieve it with RequestIDFromContext.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request ID (possibly empty).
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// WithLogger derives a request-scoped logger and attaches it to the context.
// It also emits an access log entry on completion with method, path, status,
// duration, remote IP, and request ID. Request bodies are never logged.
func WithLogger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := RequestIDFromContext(r.Context())
			reqLogger := base.With().
				Str("request_id", reqID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Logger()
			ctx := context.WithValue(r.Context(), ctxKeyLogger, reqLogger)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r.WithContext(ctx))
			dur := time.Since(start)

			remote := clientIP(r)
			reqLogger.Info().
				Int("status", rec.status).
				Dur("duration_ms", dur).
				Str("remote_ip", remote).
				Int64("bytes", rec.bytes).
				Msg("http_request")
		})
	}
}

// LoggerFromContext returns the request-scoped logger (or a no-op logger if
// none was installed).
func LoggerFromContext(ctx context.Context) zerolog.Logger {
	if l, ok := ctx.Value(ctxKeyLogger).(zerolog.Logger); ok {
		return l
	}
	return zerolog.Nop()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wrote {
		return
	}
	s.status = code
	s.wrote = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

// SecurityHeaders sets headers appropriate for the SPA + API origin.
// Strict-Transport-Security is only added when TLS is active (tlsActive=true).
func SecurityHeaders(tlsActive bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// CSP permits only same-origin assets plus inline styles (needed by
			// Tailwind/shadcn in the SPA build). No eval, no cross-origin.
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; connect-src 'self' ws: wss:; font-src 'self' data:; "+
					"frame-ancestors 'none'; base-uri 'self'")
			if tlsActive {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodyBytes wraps r.Body in http.MaxBytesReader to prevent unbounded
// request bodies. A zero or negative limit disables the cap.
func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BearerAuth enforces a bearer token on every request except explicitly
// allow-listed prefixes (health, metrics). Failures increment the per-IP
// failure counter used by AuthRateLimiter.
//
// When expected == "" this middleware is a no-op - intended for tests or
// local-only development modes.
func BearerAuth(expected string, allowPrefixes []string, failLimiter *AuthRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range allowPrefixes {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			ip := clientIP(r)
			if failLimiter != nil && failLimiter.Blocked(ip) {
				http.Error(w, "too many failed auth attempts", http.StatusTooManyRequests)
				return
			}
			h := r.Header.Get("Authorization")
			const pfx = "Bearer "
			if !strings.HasPrefix(h, pfx) {
				authFail(w, failLimiter, ip, "missing bearer token")
				return
			}
			tok := strings.TrimSpace(h[len(pfx):])
			if subtle.ConstantTimeCompare([]byte(tok), []byte(expected)) != 1 {
				authFail(w, failLimiter, ip, "invalid bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authFail(w http.ResponseWriter, lim *AuthRateLimiter, ip, msg string) {
	if lim != nil {
		lim.RecordFailure(ip)
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="mfpoc"`)
	http.Error(w, msg, http.StatusUnauthorized)
}

// clientIP extracts the remote address, preferring X-Forwarded-For when set.
// It is primarily used for rate-limit bucketing, not for audit truth.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = xff[:idx]
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimiter is a simple fixed-window per-IP limiter appropriate for a
// single-process, single-droplet deployment. It is intentionally naive: this
// is not a distributed rate limiter and does not need to be.
type RateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	limit    int
	counters map[string]*ipCounter
}

type ipCounter struct {
	count     int
	windowEnd time.Time
}

// NewRateLimiter constructs a limiter that allows `limit` requests per
// `window`. A limit of zero disables rate limiting.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		window:   window,
		limit:    limit,
		counters: make(map[string]*ipCounter),
	}
}

// Allow reports whether an IP may proceed and increments its counter.
func (rl *RateLimiter) Allow(ip string) bool {
	if rl == nil || rl.limit <= 0 {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	c, ok := rl.counters[ip]
	if !ok || now.After(c.windowEnd) {
		rl.counters[ip] = &ipCounter{count: 1, windowEnd: now.Add(rl.window)}
		return true
	}
	if c.count >= rl.limit {
		return false
	}
	c.count++
	return true
}

// Middleware returns an http middleware using this limiter. Only methods in
// the `methods` set are limited; others pass through. Passing an empty set
// limits every method.
func (rl *RateLimiter) Middleware(methods map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(methods) == 0 || methods[r.Method] {
				if !rl.Allow(clientIP(r)) {
					w.Header().Set("Retry-After", "60")
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthRateLimiter tracks failed auth attempts per IP with exponential backoff
// semantics: once the threshold is crossed in a window, the IP is blocked
// for the remainder of that window.
type AuthRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*authBucket
}

type authBucket struct {
	failures  int
	blockedAt time.Time
}

// NewAuthRateLimiter constructs the limiter. Limit 0 disables it.
func NewAuthRateLimiter(limit int, window time.Duration) *AuthRateLimiter {
	return &AuthRateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*authBucket),
	}
}

// RecordFailure increments the failure counter for an IP.
func (a *AuthRateLimiter) RecordFailure(ip string) {
	if a == nil || a.limit <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	b, ok := a.buckets[ip]
	if !ok || now.Sub(b.blockedAt) > a.window {
		a.buckets[ip] = &authBucket{failures: 1, blockedAt: now}
		return
	}
	b.failures++
}

// Blocked reports whether the IP has crossed the threshold within the window.
func (a *AuthRateLimiter) Blocked(ip string) bool {
	if a == nil || a.limit <= 0 {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.buckets[ip]
	if !ok {
		return false
	}
	if time.Since(b.blockedAt) > a.window {
		delete(a.buckets, ip)
		return false
	}
	return b.failures >= a.limit
}
