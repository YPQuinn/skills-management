// Package httpapi serves the versioned REST boundary and the embedded WebUI
// on the loopback interface only, with strict Host/Origin validation, JSON
// error envelopes, and API/SPA isolation.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skillctl/internal/bootstrap"
	"skillctl/internal/webui"
)

// Codes for HTTP-boundary errors; application errors keep their app codes.
const (
	codeForbidden        = "forbidden"
	codeBadRequest       = "bad_request"
	codeUnsupportedMedia = "unsupported_media_type"
	codeNotFound         = "not_found"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

// Server is the HTTP adapter for one bootstrap manager.
type Server struct {
	bm   *bootstrap.Manager
	port int
}

// New returns a Server whose loopback origin is 127.0.0.1:port, the port the
// listener was actually bound to.
func New(bm *bootstrap.Manager, port int) *Server {
	return &Server{bm: bm, port: port}
}

// Handler builds the chi router: versioned /api/v1 routes in front of the
// embedded SPA, with API isolation so unknown API paths never receive the
// SPA shell. The loopback Host/Origin guard wraps every route, so the SPA
// and static tree are just as isolated as the API.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders)
	r.Use(s.loopbackGuard)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/status", s.handleStatus)
		r.With(jsonMutationsOnly).Post("/setup", s.handleSetup)
		r.NotFound(handleAPINotFound)
		r.MethodNotAllowed(handleAPINotFound)
	})
	r.NotFound(webui.Handler)
	return r
}

// Serve serves the handler on the bound listener until ctx is cancelled,
// then shuts down gracefully.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// loopbackGuard accepts only the exact loopback Host carrying the actual
// bound port, and an Origin that exactly matches the current loopback
// origin. It wraps the whole router: the SPA, static assets, and deep links
// are as protected as the API.
func (s *Server) loopbackGuard(next http.Handler) http.Handler {
	host := fmt.Sprintf("127.0.0.1:%d", s.port)
	origin := "http://" + host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			emitError(w, codeForbidden, "only loopback access is allowed", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != origin {
			emitError(w, codeForbidden, "invalid origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// jsonMutationsOnly requires application/json for mutating methods. It is
// applied to known mutation routes only, so unknown API paths keep their
// JSON 404 instead of a 415 from a missing Content-Type.
func jsonMutationsOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mt != "application/json" {
				emitError(w, codeUnsupportedMedia, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
