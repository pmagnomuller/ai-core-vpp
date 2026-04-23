FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.work ./
COPY producer/go.mod producer/go.sum* ./producer/
COPY orchestrator/go.mod orchestrator/go.sum* ./orchestrator/

# Download deps first for better caching.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd orchestrator && go mod download

COPY orchestrator ./orchestrator
# Producer module must be present because of the go.work reference, even though
# we don't need to compile it.
COPY producer/main.go ./producer/main.go

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd orchestrator && CGO_ENABLED=0 go build -o /out/orchestrator .

FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build /out/orchestrator /usr/local/bin/orchestrator
USER app
ENTRYPOINT ["/usr/local/bin/orchestrator"]
