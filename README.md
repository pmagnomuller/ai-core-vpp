# VPP-Core: Mini Virtual Power Plant Orchestrator

VPP-Core is an MVP that demonstrates:
- high-concurrency IoT telemetry ingestion in Go
- event-driven processing with Kafka + Benthos + n8n
- AI-assisted optimization via a Python gRPC service
- a small operator UI to observe simulation outputs live

## Architecture (MVP)

`producer (Go)` -> `Redpanda/Kafka (telemetry.raw)` -> `Benthos` -> `Postgres (telemetry_events + alerts)` + `n8n webhook`

`orchestrator (Go REST)` -> `optimizer (Python gRPC + OpenAI)`

## Services

- `redpanda`: Kafka-compatible broker
- `postgres`: app data (`vpp`) + n8n DB (`n8n`)
- `n8n`: low-code workflow endpoint for low-battery events
- `producer`: simulated smart-home device telemetry generator
- `benthos`: stream transform/routing pipeline
- `optimizer`: Python service that generates optimization plans
- `orchestrator`: REST API (`/healthz`, `/devices/{id}/latest`, `/devices/{id}/strategy`, `/alerts`)
- `ui`: React operator console (`http://localhost:3000`)

## What this simulation actually does

1. `producer` simulates many smart homes sending telemetry continuously.
2. `benthos` enriches events and stores them in Postgres.
3. low-battery events are also persisted as alerts and pushed to n8n webhook.
4. `orchestrator` exposes read APIs over that live stream state.
5. `optimizer` uses your OpenAI key to produce a strategy on demand.
6. `ui` shows latest telemetry, recent alerts, and generated strategy for a device.

## Prerequisites

- Docker
- Go 1.23+ (for local build)
- Python 3.12+ (for local syntax checks / tooling)
- OpenAI API key

## Quick Start

```bash
cp .env.example .env
# set OPENAI_API_KEY in .env

make up
make demo
```

Then open:

- UI: `http://localhost:3000`
- n8n: `http://localhost:5678`

Stop everything:

```bash
make down
```

## Useful Make Targets

- `make up` - build and start all services
- `make down` - stop and remove containers/volumes
- `make logs` - tail logs
- `make ps` - list services
- `make proto` - regenerate Go/Python gRPC stubs
- `make build` - local Go build sanity check
- `make demo` - run end-to-end API smoke flow

## One-time n8n Workflow Import

Import `n8n/workflows/low-battery.json` in the n8n UI:
1. open `http://localhost:5678`
2. create/import workflow from file
3. activate it

Note: alerts are also persisted directly by Benthos into Postgres, so `/alerts` works even if n8n workflow import is skipped.
