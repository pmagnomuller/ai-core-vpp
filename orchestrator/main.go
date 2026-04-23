// Orchestrator exposes a small REST API that unifies the VPP stack:
//
//	GET /healthz                      - liveness
//	GET /devices/{id}/latest          - most recent telemetry row
//	GET /devices/{id}/strategy        - pulls 7d history and calls the
//	                                    Python optimizer over gRPC, returns
//	                                    the LLM-generated plan.
//	GET /alerts                       - most recent low-battery alerts
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ai-core-vpp/orchestrator/internal/api"
	"github.com/ai-core-vpp/orchestrator/internal/optimizer"
	pb "github.com/ai-core-vpp/orchestrator/gen/optimizerpb"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	httpAddr := envOr("HTTP_ADDR", ":8080")
	pgDSN := envOr("POSTGRES_DSN", "postgres://vpp:vpp@localhost:5432/vpp?sslmode=disable")
	optAddr := envOr("OPTIMIZER_ADDR", "localhost:50051")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := connectPostgres(ctx, pgDSN)
	if err != nil {
		slog.Error("postgres connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	optConn, err := grpc.NewClient(optAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("optimizer dial", "err", err)
		os.Exit(1)
	}
	defer optConn.Close()
	optClient := pb.NewOptimizerClient(optConn)

	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           api.NewRouter(pool, optimizer.NewClient(optClient)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("orchestrator listening", "addr", httpAddr, "optimizer", optAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
}

// connectPostgres opens a pgx pool with a bounded retry loop so the container
// can come up before Postgres is reachable without crash-looping.
func connectPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute

	var pool *pgxpool.Pool
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return pool, nil
			} else {
				err = pingErr
				pool.Close()
			}
		}
		slog.Warn("postgres not ready", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("postgres unreachable: %w", err)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
