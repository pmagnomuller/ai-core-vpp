# Build the producer binary from the repo root so go.work sees both modules.
FROM golang:1.23-alpine AS build
WORKDIR /src

# Bring in both modules plus go.work first to maximize layer caching.
COPY go.work ./
COPY producer/go.mod producer/go.sum* ./producer/
COPY orchestrator/go.mod orchestrator/go.sum* ./orchestrator/
COPY orchestrator/gen ./orchestrator/gen

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd producer && go mod download

COPY producer ./producer

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd producer && CGO_ENABLED=0 go build -o /out/producer .

FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build /out/producer /usr/local/bin/producer
USER app
ENTRYPOINT ["/usr/local/bin/producer"]
