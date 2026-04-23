// Package api holds the HTTP handlers for the orchestrator.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ai-core-vpp/orchestrator/internal/optimizer"
)

type Router struct {
	mux *http.ServeMux
	db  *pgxpool.Pool
	opt *optimizer.Client
}

func NewRouter(db *pgxpool.Pool, opt *optimizer.Client) http.Handler {
	r := &Router{mux: http.NewServeMux(), db: db, opt: opt}
	r.mux.HandleFunc("GET /healthz", r.healthz)
	r.mux.HandleFunc("GET /devices/{id}/latest", r.deviceLatest)
	r.mux.HandleFunc("GET /devices/{id}/strategy", r.deviceStrategy)
	r.mux.HandleFunc("GET /alerts", r.listAlerts)
	return withLogging(r.mux)
}

func (r *Router) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type telemetryRow struct {
	DeviceID        string    `json:"device_id"`
	Ts              time.Time `json:"ts"`
	KwUsage         float64   `json:"kw_usage"`
	BatterySocPct   float64   `json:"battery_soc_pct"`
	HeatpumpStatus  string    `json:"heatpump_status"`
	GridPriceEurKwh float64   `json:"grid_price_eur_kwh"`
	LowBattery      bool      `json:"low_battery"`
}

func (r *Router) deviceLatest(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing device id")
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()

	var row telemetryRow
	err := r.db.QueryRow(ctx, `
		SELECT device_id, ts, kw_usage, battery_soc_pct,
		       heatpump_status, grid_price_eur_kwh, low_battery
		FROM telemetry_events
		WHERE device_id = $1
		ORDER BY ts DESC
		LIMIT 1
	`, id).Scan(
		&row.DeviceID, &row.Ts, &row.KwUsage, &row.BatterySocPct,
		&row.HeatpumpStatus, &row.GridPriceEurKwh, &row.LowBattery,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "no telemetry for device")
		return
	}
	if err != nil {
		slog.Error("deviceLatest query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (r *Router) deviceStrategy(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing device id")
		return
	}

	// Pull up to ~7 days of history. In this MVP that's bounded by row count
	// rather than strict time range to keep the query cheap.
	ctx, cancel := context.WithTimeout(req.Context(), 60*time.Second)
	defer cancel()

	rows, err := r.db.Query(ctx, `
		SELECT ts, kw_usage, battery_soc_pct, heatpump_status, grid_price_eur_kwh
		FROM telemetry_events
		WHERE device_id = $1 AND ts > NOW() - INTERVAL '7 days'
		ORDER BY ts ASC
		LIMIT 2016
	`, id)
	if err != nil {
		slog.Error("deviceStrategy query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var history []optimizer.TelemetryPoint
	for rows.Next() {
		var (
			ts    time.Time
			kw    float64
			soc   float64
			hp    string
			price float64
		)
		if err := rows.Scan(&ts, &kw, &soc, &hp, &price); err != nil {
			slog.Error("deviceStrategy scan", "err", err)
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		history = append(history, optimizer.TelemetryPoint{
			Ts:              ts.UTC().Format(time.RFC3339),
			KwUsage:         kw,
			BatterySocPct:   soc,
			HeatpumpStatus:  hp,
			GridPriceEurKwh: price,
		})
	}
	if err := rows.Err(); err != nil {
		slog.Error("deviceStrategy rows", "err", err)
		writeError(w, http.StatusInternalServerError, "iteration failed")
		return
	}
	if len(history) == 0 {
		writeError(w, http.StatusNotFound, "no telemetry for device in last 7 days")
		return
	}

	plan, err := r.opt.OptimizeStrategy(ctx, id, history)
	if err != nil {
		slog.Error("optimizer call", "err", err)
		writeError(w, http.StatusBadGateway, "optimizer error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": id,
		"samples":   len(history),
		"plan":      plan,
	})
}

type alertRow struct {
	ID        int64           `json:"id"`
	DeviceID  string          `json:"device_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func (r *Router) listAlerts(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()

	rows, err := r.db.Query(ctx, `
		SELECT id, device_id, kind, payload, created_at
		FROM alerts
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		slog.Error("listAlerts query", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := make([]alertRow, 0, 100)
	for rows.Next() {
		var a alertRow
		if err := rows.Scan(&a.ID, &a.DeviceID, &a.Kind, &a.Payload, &a.CreatedAt); err != nil {
			slog.Error("listAlerts scan", "err", err)
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func withLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		h.ServeHTTP(sw, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
