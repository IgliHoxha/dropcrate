// Package api exposes the HTTP interface for dropcrate.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/IgliHoxha/dropcrate/internal/auth"
	"github.com/IgliHoxha/dropcrate/internal/metrics"
	"github.com/IgliHoxha/dropcrate/internal/service"
	"github.com/IgliHoxha/dropcrate/internal/urlsign"
)

// ReadyFunc reports whether the service's dependencies are reachable. It backs
// the /readyz probe; a nil error means ready.
type ReadyFunc func(context.Context) error

// Options configures an API handler set. All fields are optional: a nil Ready
// makes /readyz liveness-only, a disabled Auth leaves mutations open, and a
// disabled Signer leaves downloads open by id.
type Options struct {
	BaseURL        string
	MaxUploadBytes int64
	Ready          ReadyFunc
	Auth           *auth.Authenticator
	Signer         *urlsign.Signer
}

// API holds the dependencies shared by every handler.
type API struct {
	svc            *service.Service
	log            *slog.Logger
	baseURL        string
	maxUploadBytes int64
	ready          ReadyFunc
	auth           *auth.Authenticator
	signer         *urlsign.Signer
}

// New builds an API handler set from opts.
func New(svc *service.Service, log *slog.Logger, opts Options) *API {
	return &API{
		svc:            svc,
		log:            log,
		baseURL:        opts.BaseURL,
		maxUploadBytes: opts.MaxUploadBytes,
		ready:          opts.Ready,
		auth:           opts.Auth,
		signer:         opts.Signer,
	}
}

// Router returns the fully wired chi router.
func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// middleware.RealIP is intentionally omitted: it rewrites RemoteAddr from
	// client-supplied X-Forwarded-For / X-Real-IP headers, which are spoofable
	// unless a trusted proxy sets them. Add it back behind such a proxy if the
	// real client IP is ever needed.
	r.Use(middleware.Recoverer)
	r.Use(metricsMiddleware)
	r.Use(requestLogger(a.log))

	r.Handle("/metrics", metrics.Handler())
	r.Get("/healthz", a.handleHealth)
	r.Get("/readyz", a.handleReady)

	r.Route("/v1/files", func(r chi.Router) {
		// Reads stay open so shareable download links keep working; mutations
		// require an API key when authentication is enabled.
		r.Get("/{id}", a.handleDownload)
		r.Get("/{id}/meta", a.handleMetadata)

		r.Group(func(r chi.Router) {
			r.Use(a.requireAuth)
			r.Post("/", a.handleUpload)
			r.Delete("/{id}", a.handleDelete)
		})
	})

	return r
}

// requireAuth rejects requests without a valid bearer API key. It is a no-op
// when authentication is disabled (no keys configured).
func (a *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.auth == nil || !a.auth.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		if !a.auth.Valid(auth.BearerToken(r.Header.Get("Authorization"))) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "missing or invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON serializes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError emits a consistent JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
