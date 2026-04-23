// Producer simulates a fleet of smart-home IoT devices (heat pumps + home
// batteries) and streams telemetry as JSON into a Kafka topic.
//
// Each simulated device runs in its own goroutine and ticks at a configurable
// rate. Values are jittered so the downstream LLM has something interesting to
// optimize: battery SoC drifts over the day, heat pumps flip between heating
// and idle, and grid price follows a cheap day-ahead curve.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/segmentio/kafka-go"
)

type telemetry struct {
	DeviceID        string  `json:"device_id"`
	Ts              string  `json:"ts"`
	KwUsage         float64 `json:"kw_usage"`
	BatterySocPct   float64 `json:"battery_soc_pct"`
	HeatpumpStatus  string  `json:"heatpump_status"`
	GridPriceEurKwh float64 `json:"grid_price_eur_kwh"`
}

var (
	eventsProduced = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vpp_producer_events_produced_total",
		Help: "Total telemetry events successfully handed to the Kafka async producer.",
	})
	produceErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "vpp_producer_produce_errors_total",
		Help: "Total errors returned by the Kafka async producer.",
	})
)

func main() {
	var (
		brokers   = flag.String("brokers", envOr("KAFKA_BROKERS", "localhost:19092"), "comma-separated Kafka brokers")
		topic     = flag.String("topic", envOr("KAFKA_TOPIC", "telemetry.raw"), "Kafka topic")
		devices   = flag.Int("devices", envInt("PRODUCER_DEVICES", 1000), "number of simulated devices")
		rateHz    = flag.Float64("rate", envFloat("PRODUCER_RATE_HZ", 1.0), "events per second per device")
		metricsAd = flag.String("metrics-addr", envOr("METRICS_ADDR", ":9101"), "address for /metrics endpoint")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("starting producer",
		"brokers", *brokers,
		"topic", *topic,
		"devices", *devices,
		"rate_hz", *rateHz,
	)

	writer, err := newWriter(strings.Split(*brokers, ","), *topic)
	if err != nil {
		slog.Error("failed to create writer", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := writer.Close(); err != nil {
			slog.Error("writer close", "err", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go serveMetrics(ctx, *metricsAd)

	var wg sync.WaitGroup
	for i := 0; i < *devices; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runDevice(ctx, writer, deviceID(id), *rateHz)
		}(i)
	}

	<-ctx.Done()
	slog.Info("shutdown signal received, waiting for devices to drain")
	wg.Wait()
	slog.Info("bye")
}

func newWriter(brokers []string, topic string) (*kafka.Writer, error) {
	// Validate connectivity early so failures are visible on startup.
	conn, err := kafka.DialContext(context.Background(), "tcp", brokers[0])
	if err != nil {
		return nil, fmt.Errorf("kafka dial failed: %w", err)
	}
	_ = conn.Close()

	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireOne,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 50 * time.Millisecond,
		BatchSize:    256,
		Async:        false,
	}, nil
}

func serveMetrics(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	slog.Info("metrics listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("metrics server", "err", err)
	}
}

// runDevice simulates a single smart home. It maintains lightweight local
// state (battery SoC, heat pump mode) and ticks at `rateHz` events per second.
func runDevice(ctx context.Context, w *kafka.Writer, id string, rateHz float64) {
	if rateHz <= 0 {
		rateHz = 1
	}
	interval := time.Duration(float64(time.Second) / rateHz)
	// Jitter the first tick so 1000 goroutines don't all fire on the same ms.
	jitter := time.Duration(rand.Int64N(int64(interval)))
	t := time.NewTimer(jitter)
	defer t.Stop()

	st := newDeviceState(id)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		t.Reset(interval)

		st.step()
		payload, err := json.Marshal(st.toTelemetry())
		if err != nil {
			slog.Error("marshal telemetry", "err", err)
			continue
		}
		msg := kafka.Message{
			Key:   []byte(id),
			Value: payload,
			Time:  time.Now().UTC(),
		}
		if err := w.WriteMessages(ctx, msg); err != nil {
			produceErrors.Inc()
			slog.Error("kafka write error", "device_id", id, "err", err)
		} else {
			eventsProduced.Inc()
		}
	}
}

type deviceState struct {
	id             string
	batterySoc     float64
	heatpumpStatus string
	// rng is a per-device PRNG so goroutines never touch a shared RNG.
	rng *rand.Rand
}

func newDeviceState(id string) *deviceState {
	seed := rand.Uint64()
	return &deviceState{
		id:             id,
		batterySoc:     30 + rand.Float64()*60, // start between 30% and 90%
		heatpumpStatus: "idle",
		rng:            rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)),
	}
}

func (s *deviceState) step() {
	// Heat pump flips ~once every 60 ticks with some randomness.
	if s.rng.Float64() < 1.0/60.0 {
		switch s.heatpumpStatus {
		case "idle":
			s.heatpumpStatus = "heating"
		default:
			s.heatpumpStatus = "idle"
		}
	}
	// Battery drifts: charges slightly when heat pump is idle, drains otherwise.
	delta := (s.rng.Float64() - 0.5) * 0.2
	if s.heatpumpStatus == "idle" {
		delta += 0.15
	} else {
		delta -= 0.25
	}
	s.batterySoc = clamp(s.batterySoc+delta, 0, 100)
}

func (s *deviceState) toTelemetry() telemetry {
	kw := 0.2 + s.rng.Float64()*0.5
	if s.heatpumpStatus == "heating" {
		kw += 2.0 + s.rng.Float64()*1.0
	}
	return telemetry{
		DeviceID:        s.id,
		Ts:              time.Now().UTC().Format(time.RFC3339),
		KwUsage:         round2(kw),
		BatterySocPct:   round2(s.batterySoc),
		HeatpumpStatus:  s.heatpumpStatus,
		GridPriceEurKwh: round3(gridPriceAt(time.Now())),
	}
}

// gridPriceAt returns a plausible day-ahead price curve: cheap at night,
// expensive during morning and evening peaks.
func gridPriceAt(t time.Time) float64 {
	h := float64(t.UTC().Hour()) + float64(t.UTC().Minute())/60.0
	// Two Gaussian peaks at 08:00 and 19:00 on a 0.10 EUR/kWh baseline.
	peak1 := 0.20 * math.Exp(-math.Pow((h-8)/1.5, 2))
	peak2 := 0.30 * math.Exp(-math.Pow((h-19)/1.5, 2))
	return 0.10 + peak1 + peak2
}

func deviceID(i int) string {
	return "home-" + padZero(i, 5)
}

func padZero(i, width int) string {
	s := strconv.Itoa(i)
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}
