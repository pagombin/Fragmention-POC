package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestBearerAuth_Success(t *testing.T) {
	mw := BearerAuth("secret-token", []string{"/healthz"}, nil)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestBearerAuth_Fail(t *testing.T) {
	limiter := NewAuthRateLimiter(2, time.Minute)
	mw := BearerAuth("secret-token", nil, limiter)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")
}

func TestBearerAuth_AllowPrefix(t *testing.T) {
	mw := BearerAuth("secret-token", []string{"/healthz"}, nil)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestBearerAuth_RateLimitsFailures(t *testing.T) {
	limiter := NewAuthRateLimiter(2, time.Minute)
	mw := BearerAuth("secret", nil, limiter)(okHandler())
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		req.Header.Set("Authorization", "Bearer bad")
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	// Third attempt should now be throttled.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	req.Header.Set("Authorization", "Bearer secret") // correct, but IP is blocked
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	require.True(t, rl.Allow("ip"))
	require.True(t, rl.Allow("ip"))
	require.False(t, rl.Allow("ip"))
	// Different IP gets its own bucket.
	require.True(t, rl.Allow("other"))
}

func TestRequestID_Propagates(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, "abc", seen)
	require.Equal(t, "abc", rec.Header().Get("X-Request-ID"))
}

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.NotEmpty(t, seen)
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(true)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	require.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age")
}

func TestSecurityHeaders_NoHSTSWithoutTLS(t *testing.T) {
	h := SecurityHeaders(false)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Empty(t, rec.Header().Get("Strict-Transport-Security"))
}

func TestMaxBodyBytes(t *testing.T) {
	var gotErr error
	h := MaxBodyBytes(5)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64)
		_, gotErr = r.Body.Read(buf)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 100)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Error(t, gotErr) // MaxBytesReader trips
}

func TestLoggerInContext(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)
	wrapped := RequestID(WithLogger(base)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := LoggerFromContext(r.Context())
		l.Info().Msg("handler")
		w.WriteHeader(http.StatusTeapot)
	})))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTeapot, rec.Code)
	out := buf.String()
	require.Contains(t, out, "handler")
	require.Contains(t, out, "http_request")
}
