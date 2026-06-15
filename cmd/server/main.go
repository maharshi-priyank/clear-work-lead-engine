package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/amplexo/clearwork-leads-engine/internal/campaign"
	"github.com/amplexo/clearwork-leads-engine/internal/db"
	"github.com/amplexo/clearwork-leads-engine/internal/leads"
	mw "github.com/amplexo/clearwork-leads-engine/internal/middleware"
	"github.com/amplexo/clearwork-leads-engine/internal/providers"
	"github.com/amplexo/clearwork-leads-engine/internal/vault"
)

// corsMiddleware reads allowed origins from the CORS_ORIGIN env var
// (comma-separated list, e.g. "https://leads.getclearwork.in,http://localhost:5173").
func corsMiddleware(next http.Handler) http.Handler {
	rawOrigins := os.Getenv("CORS_ORIGIN")
	allowed := make(map[string]bool)
	for _, o := range strings.Split(rawOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = true
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowed[origin] || len(allowed) == 0) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-User-ID")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	_ = godotenv.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Database ──────────────────────────────────────────────────────────
	pool, err := db.NewPool(ctx)
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")

	// ── Redis ─────────────────────────────────────────────────────────────
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	parsedRedis, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("invalid REDIS_URL", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(parsedRedis)
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis ping failed — jobs will be queued locally", "err", err)
	} else {
		slog.Info("redis connected", "addr", parsedRedis.Addr)
	}
	rdb.Close()

	redisOpt := asynq.RedisClientOpt{
		Addr:     parsedRedis.Addr,
		Username: parsedRedis.Username,
		Password: parsedRedis.Password,
		DB:       parsedRedis.DB,
		TLSConfig: parsedRedis.TLSConfig,
	}
	queueClient := asynq.NewClient(redisOpt)
	defer queueClient.Close()

	// ── Registry & services ───────────────────────────────────────────────
	registry    := providers.NewRegistry()
	vaultSvc    := vault.NewService(pool)
	campaignSvc := campaign.NewService(pool, queueClient)
	leadsSvc    := leads.NewService(pool)
	worker      := campaign.NewWorker(pool, registry, vaultSvc)

	// ── Asynq worker server (runs in background goroutine) ────────────────
	asynqSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 20,
		Queues:      map[string]int{"campaigns": 1},
	})
	workerMux := asynq.NewServeMux()
	workerMux.HandleFunc(campaign.TaskCampaignRun, worker.ProcessCampaign)
	go func() {
		if err := asynqSrv.Run(workerMux); err != nil {
			slog.Error("asynq worker failed", "err", err)
		}
	}()
	slog.Info("asynq worker started", "concurrency", 20)

	// ── HTTP router ───────────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(corsMiddleware)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Group(func(r chi.Router) {
		r.Use(mw.InjectUserID)

		r.Mount("/campaigns", campaign.NewHandler(campaignSvc).Routes())
		r.Mount("/leads", leads.NewHandler(leadsSvc).Routes())
		r.Mount("/vault", vault.NewHandler(vaultSvc, registry).Routes())

		// Convenience: GET /campaigns/:id with embedded leads (used by results page)
		r.Get("/campaigns/{id}/full", func(w http.ResponseWriter, r *http.Request) {
			uid := mw.UserID(r.Context())
			id := chi.URLParam(r, "id")

			c, err := campaignSvc.FindOne(r.Context(), uid, id)
			if err != nil {
				http.Error(w, `{"error":"campaign not found"}`, 404)
				return
			}
			result, _ := leadsSvc.FindAll(r.Context(), uid, leads.QueryParams{
				CampaignID: id, Page: 1, Limit: 1000,
			})
			type resp struct {
				*campaign.Campaign
				Leads any `json:"leads"`
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"data": resp{
				Campaign: c,
				Leads:    result.Items,
			}})
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("leads engine listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
	asynqSrv.Shutdown()
	slog.Info("stopped")
}
