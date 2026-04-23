#!/usr/bin/env bash
# End-to-end smoke test for VPP-Core.
#
# Assumes `make up` has been run. Waits for telemetry to accumulate, then
# exercises every hop in the pipeline: producer -> Redpanda -> Benthos ->
# Postgres + n8n -> orchestrator -> Python optimizer -> OpenAI.

set -euo pipefail

ORCH=${ORCH:-http://localhost:8080}
DEVICE=${DEVICE:-home-00042}
WAIT_SECS=${WAIT_SECS:-30}

say() { printf "\n\033[1;36m==> %s\033[0m\n" "$*"; }

say "Waiting ${WAIT_SECS}s for telemetry to flow through the pipeline..."
sleep "${WAIT_SECS}"

say "Orchestrator health"
curl -sS "${ORCH}/healthz" | jq .

say "Latest telemetry for ${DEVICE}"
curl -sS "${ORCH}/devices/${DEVICE}/latest" | jq .

say "Recent low-battery alerts (first 5)"
curl -sS "${ORCH}/alerts" | jq '.[:5]'

say "Asking the LLM for an optimization strategy for ${DEVICE}"
curl -sS "${ORCH}/devices/${DEVICE}/strategy" | jq .

say "Done."
