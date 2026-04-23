"""VPP-Core optimizer gRPC server.

Receives a device's recent telemetry history via gRPC and returns an
LLM-generated energy plan (executive summary + a list of concrete actions).

Runs two listeners in the same process:
  * gRPC on :50051 (the contract)
  * HTTP on :8081 for /healthz (container liveness probes)
"""

from __future__ import annotations

import logging
import os
import signal
import sys
import threading
from concurrent import futures
from typing import List

import grpc
import uvicorn
from fastapi import FastAPI
from langchain_core.prompts import ChatPromptTemplate
from langchain_openai import ChatOpenAI
from pydantic import BaseModel, Field

# Ensure we can import the generated stubs that live under ./gen.
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "gen"))

import optimizer_pb2  # noqa: E402  (generated)
import optimizer_pb2_grpc  # noqa: E402  (generated)

LOG = logging.getLogger("optimizer")


# ---------------------------------------------------------------------------
# LLM schema (kept intentionally small so gpt-4o-mini can nail structured output)
# ---------------------------------------------------------------------------
class PlannedAction(BaseModel):
    when: str = Field(description="ISO-8601 instant or range like '02:00-04:00'.")
    what: str = Field(description="One of: charge_battery, discharge_battery, run_heatpump, idle_heatpump.")
    why: str = Field(description="One short sentence justifying the action.")


class OptimizationPlan(BaseModel):
    strategy: str = Field(description="One-paragraph executive summary of the plan.")
    actions: List[PlannedAction] = Field(description="Ordered list of actions for the next 24 hours.")


PROMPT = ChatPromptTemplate.from_messages(
    [
        (
            "system",
            (
                "You are the optimization brain of a Virtual Power Plant. "
                "Given a single smart home's recent telemetry (battery SoC, "
                "heat pump status, household draw, day-ahead grid price), "
                "produce a short energy-management plan for the next 24h. "
                "Prefer charging the battery when grid_price_eur_kwh is low "
                "(usually 01:00-05:00 UTC) and pre-heating the home before "
                "price peaks. Always return valid structured output."
            ),
        ),
        (
            "human",
            (
                "Device: {device_id}\n"
                "Samples ({n_samples}):\n"
                "{samples}\n\n"
                "Return an OptimizationPlan."
            ),
        ),
    ]
)


def _build_chain():
    model = os.getenv("OPENAI_MODEL", "gpt-4o-mini")
    # temperature=0 keeps the demo output stable enough to screenshot.
    llm = ChatOpenAI(model=model, temperature=0)
    return PROMPT | llm.with_structured_output(OptimizationPlan)


def _format_samples(history) -> str:
    # Compact, token-efficient table. We cap samples to avoid absurd prompts;
    # the orchestrator already limits to last 7 days but an aggressive client
    # could send more.
    cap = 200
    rows = list(history)[-cap:]
    lines = ["ts,kw,soc%,hp,eur/kwh"]
    for r in rows:
        lines.append(
            f"{r.ts},{r.kw_usage:.2f},{r.battery_soc_pct:.1f},"
            f"{r.heatpump_status},{r.grid_price_eur_kwh:.3f}"
        )
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# gRPC service
# ---------------------------------------------------------------------------
class OptimizerServicer(optimizer_pb2_grpc.OptimizerServicer):
    def __init__(self) -> None:
        self._chain = _build_chain()

    def OptimizeStrategy(self, request, context):  # noqa: N802 (gRPC convention)
        if not request.history:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "history must not be empty")
        LOG.info(
            "OptimizeStrategy device=%s samples=%d",
            request.device_id,
            len(request.history),
        )
        try:
            plan: OptimizationPlan = self._chain.invoke(
                {
                    "device_id": request.device_id,
                    "n_samples": len(request.history),
                    "samples": _format_samples(request.history),
                }
            )
        except Exception as exc:  # noqa: BLE001
            LOG.exception("LLM call failed")
            context.abort(grpc.StatusCode.INTERNAL, f"LLM error: {exc}")

        return optimizer_pb2.OptimizeResponse(
            strategy=plan.strategy,
            actions=[
                optimizer_pb2.Action(when=a.when, what=a.what, why=a.why)
                for a in plan.actions
            ],
        )


def _serve_grpc(port: int) -> grpc.Server:
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=16))
    optimizer_pb2_grpc.add_OptimizerServicer_to_server(OptimizerServicer(), server)
    server.add_insecure_port(f"[::]:{port}")
    server.start()
    LOG.info("gRPC listening on :%d", port)
    return server


def _http_app() -> FastAPI:
    app = FastAPI(title="VPP-Core Optimizer", version="0.1.0")

    @app.get("/healthz")
    def healthz():
        return {"status": "ok"}

    return app


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    if not os.getenv("OPENAI_API_KEY"):
        LOG.warning("OPENAI_API_KEY is not set; OpenAI calls will fail at request time")

    grpc_port = int(os.getenv("GRPC_PORT", "50051"))
    http_port = int(os.getenv("HTTP_PORT", "8081"))

    grpc_server = _serve_grpc(grpc_port)

    # Run the HTTP server in a background thread so signal handling stays
    # with the main thread.
    config = uvicorn.Config(_http_app(), host="0.0.0.0", port=http_port, log_level="info")
    http_server = uvicorn.Server(config)
    http_thread = threading.Thread(target=http_server.run, daemon=True)
    http_thread.start()
    LOG.info("HTTP listening on :%d", http_port)

    stop = threading.Event()

    def _on_signal(signum, _frame):
        LOG.info("received signal %d, shutting down", signum)
        stop.set()

    signal.signal(signal.SIGINT, _on_signal)
    signal.signal(signal.SIGTERM, _on_signal)

    stop.wait()
    http_server.should_exit = True
    grpc_server.stop(grace=5).wait()
    LOG.info("bye")


if __name__ == "__main__":
    main()
