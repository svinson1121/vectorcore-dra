package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/config"
	"github.com/svinson1121/vectorcore-dra/internal/peermgr"
	"github.com/svinson1121/vectorcore-dra/internal/router"
)

// Server is the HTTP management API server.
type Server struct {
	ctx        context.Context // application-level context; used for Sync calls from API handlers
	cfg        *config.Config
	cfgPath    string
	mgr        *peermgr.Manager
	router     *router.Router
	log        *zap.Logger
	startAt    time.Time
	RecentMsgs *RecentMsgBuf
}

// New creates an API server. appCtx must be the application-level context (not a
// request context) so that peers started via API calls survive the request lifetime.
func New(appCtx context.Context, cfg *config.Config, cfgPath string, mgr *peermgr.Manager, r *router.Router, log *zap.Logger) *Server {
	return &Server{
		ctx:        appCtx,
		cfg:        cfg,
		cfgPath:    cfgPath,
		mgr:        mgr,
		router:     r,
		log:        log,
		startAt:    time.Now(),
		RecentMsgs: &RecentMsgBuf{},
	}
}

// Handler builds and returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.Recoverer)
	mux.Use(middleware.RealIP)
	mux.Use(zapMiddleware(s.log))

	// Huma API - all endpoints and docs under /api/v1/
	humaConfig := huma.DefaultConfig("VectorCore DRA API", "0.2.0B")
	humaConfig.OpenAPIPath = "/api/v1/openapi.json"
	humaConfig.DocsPath = "/api/v1/docs"
	humaConfig.SchemasPath = "/api/v1/schemas"
	api := humachi.New(mux, humaConfig)
	api.UseMiddleware()

	// Register all endpoint groups
	registerPeers(api, s)
	registerPeerGroups(api, s)
	registerRoutes(api, s)
	registerIMSIRoutes(api, s)
	registerRuntime(api, s)

	// Prometheus metrics - not under /api/v1
	mux.Handle("/metrics", promhttp.Handler())

	// Health probe
	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Embedded React UI at /ui/
	mux.Handle("/ui", http.RedirectHandler("/ui/", http.StatusMovedPermanently))
	ui := uiHandler()
	mux.Handle("/ui/", ui)
	mux.Handle("/ui/*", ui)

	// Redirect root to UI
	mux.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})

	return mux
}

// Start runs the HTTP server until ctx is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("API server listening", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// zapMiddleware returns a chi middleware that logs each request with zap.
func zapMiddleware(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			status := ww.Status()
			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", status),
				zap.Duration("duration", time.Since(start)),
			}
			if status >= 500 {
				log.Error("http", fields...)
			} else if status >= 400 {
				log.Warn("http", fields...)
			} else {
				log.Debug("http", fields...)
			}
		})
	}
}

// saveConfig persists the current in-memory config atomically.
func (s *Server) saveConfig() error {
	return config.Save(s.cfgPath, s.cfg)
}
