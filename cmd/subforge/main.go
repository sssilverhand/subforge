package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/sssilverhand/subforge/internal/api/admin"
	"github.com/sssilverhand/subforge/internal/api/hy2auth"
	"github.com/sssilverhand/subforge/internal/api/sub"
	"github.com/sssilverhand/subforge/internal/bot"
	"github.com/sssilverhand/subforge/web"
	"io/fs"
	"mime"
	"path"
	"strings"
	"github.com/sssilverhand/subforge/internal/config"
	"github.com/sssilverhand/subforge/internal/core/subscription"
	"github.com/sssilverhand/subforge/internal/db"
	"github.com/sssilverhand/subforge/internal/scheduler"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ─── Database ────────────────────────────────────────────────────────────
	pool, err := db.Open(ctx, cfg.Database.DSN)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	subsStore    := db.NewSubscriptionStore(pool)
	nodesStore   := db.NewNodeStore(pool)
	usersStore   := db.NewUserStore(pool)
	tokensStore  := db.NewTokenStore(pool)
	botUserStore := db.NewBotUserStore(pool)

	// ─── Services ────────────────────────────────────────────────────────────
	subSvc := subscription.NewService(subsStore, nodesStore)

	// ─── Scheduler ───────────────────────────────────────────────────────────
	sched := scheduler.New(
		subsStore, nodesStore, subSvc, log,
		cfg.Scheduler.TrafficPollInterval,
		cfg.Scheduler.ExpiryCheckInterval,
	)
	go sched.Start(ctx)

	// ─── HTTP Router ─────────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	}))

	// Public: subscription links
	r.Mount("/sub", sub.NewHandler(&subAdapter{store: subsStore, svc: subSvc}).Routes())

	// Internal: hysteria2 auth backend (bind only on loopback in production)
	r.Mount("/internal/hy2/auth", hy2auth.NewHandler(subsStore))

	// Admin API
	r.Mount("/api", admin.NewHandler(cfg, subsStore, subSvc, nodesStore, usersStore, tokensStore).Routes())

	// ─── Telegram Bot ────────────────────────────────────────────────────────
	if cfg.Bot.Enabled && cfg.Bot.Token != "" {
		tgBot, err := bot.New(cfg, subsStore, subSvc, nodesStore, usersStore, botUserStore, log)
		if err != nil {
			log.Error("create telegram bot", "err", err)
			os.Exit(1)
		}
		go tgBot.Start(ctx)
	}

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// React SPA — serve from embedded dist/
	spaFS, _ := fs.Sub(web.FS, "dist")
	r.Get("/*", spaHandler(spaFS))

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("subforge started", "addr", srv.Addr, "external", cfg.Server.ExternalURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

// ─── Sub adapter ─────────────────────────────────────────────────────────────
// Implements sub.Store by bridging SubscriptionStore + subscription.Service.

type subAdapter struct {
	store *db.SubscriptionStore
	svc   *subscription.Service
}

func (a *subAdapter) GetSubscriptionByToken(token string) (*sub.Subscription, error) {
	ctx := context.Background()
	dbSub, err := a.store.GetByToken(ctx, token)
	if err != nil || dbSub == nil {
		return nil, err
	}

	data, err := a.svc.BuildSubData(ctx, dbSub)
	if err != nil {
		return nil, err
	}

	var limitBytes int64
	if dbSub.TrafficLimitBytes != nil {
		limitBytes = *dbSub.TrafficLimitBytes
	}

	return &sub.Subscription{
		ID:                dbSub.ID.String(),
		Name:              derefStr(dbSub.Name),
		UserUUID:          dbSub.UUID,
		Hy2Password:       dbSub.Hy2Password,
		IsEnabled:         dbSub.IsEnabled && !dbSub.IsExpired && !dbSub.IsTrafficExceeded,
		TrafficUsedBytes:  dbSub.TrafficUsedBytes,
		TrafficLimitBytes: limitBytes,
		ExpiresAt:         dbSub.ExpiresAt,
		Endpoints:         data.Endpoints,
	}, nil
}

func (a *subAdapter) UpdateLastUsed(id string, t time.Time) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid subscription id %q: %w", id, err)
	}
	return a.store.UpdateLastUsed(context.Background(), uid, t)
}

// spaHandler serves the React SPA and falls back to index.html for client-side routing.
func spaHandler(spaFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		f, err := spaFS.Open(p)
		if err != nil || p == "" {
			// Fall back to index.html for SPA routing
			data, err2 := fs.ReadFile(spaFS, "index.html")
			if err2 != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		f.Close()
		ext := path.Ext(p)
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		http.ServeFileFS(w, r, spaFS, p)
	}
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
