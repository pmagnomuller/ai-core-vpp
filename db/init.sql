-- Postgres bootstrap for VPP-Core.
--
-- Two logical databases share this Postgres instance:
--   * vpp   : application data (telemetry_events, alerts)
--   * n8n   : n8n's own workflow/state storage
--
-- n8n expects to own its database entirely, so we keep it separate.

CREATE DATABASE n8n;

\connect vpp

CREATE TABLE IF NOT EXISTS telemetry_events (
    id                 BIGSERIAL PRIMARY KEY,
    device_id          TEXT        NOT NULL,
    ts                 TIMESTAMPTZ NOT NULL,
    kw_usage           DOUBLE PRECISION NOT NULL,
    battery_soc_pct    DOUBLE PRECISION NOT NULL,
    heatpump_status    TEXT        NOT NULL,
    grid_price_eur_kwh DOUBLE PRECISION NOT NULL,
    low_battery        BOOLEAN     NOT NULL DEFAULT FALSE,
    ingested_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS telemetry_events_device_ts_idx
    ON telemetry_events (device_id, ts DESC);

CREATE TABLE IF NOT EXISTS alerts (
    id          BIGSERIAL PRIMARY KEY,
    device_id   TEXT        NOT NULL,
    kind        TEXT        NOT NULL,
    payload     JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS alerts_device_created_idx
    ON alerts (device_id, created_at DESC);
