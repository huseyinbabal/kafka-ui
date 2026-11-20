package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/auth"
)

const sessionCookie = "kui_session"

type ctxKey int

const principalKey ctxKey = 1

func principalFrom(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey).(auth.Principal)
	return p, ok
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush keeps SSE working through the recorder.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start).Round(time.Millisecond))
	})
}

func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("panic", "err", v, "stack", string(debug.Stack()))
				writeError(w, r, apierr.New(http.StatusInternalServerError, "INTERNAL", "internal error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// requireAuth resolves the session cookie and enforces CSRF header on mutations.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, r, apierr.ErrUnauthorized)
			return
		}
		p, err := s.auth.Authenticate(r.Context(), c.Value)
		if err != nil {
			writeError(w, r, apierr.ErrUnauthorized)
			return
		}
		if isMutation(r.Method) && r.Header.Get("X-Requested-With") != "kafka-ui" {
			writeError(w, r, apierr.ErrCSRF)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

// admin wraps a handler that requires the admin role.
func admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalFrom(r.Context())
		if !ok || !p.CanWrite() {
			writeError(w, r, apierr.ErrForbidden)
			return
		}
		h(w, r)
	}
}

func isMutation(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
