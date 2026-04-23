SHELL := /bin/bash
.DEFAULT_GOAL := help

## help: show this help
help:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## up: build and start the whole stack
up: ## build and start the whole stack in the background
	@if [ ! -f .env ]; then cp .env.example .env && echo "created .env from .env.example - edit OPENAI_API_KEY"; fi
	docker compose up -d --build

## down: stop the stack and drop volumes
down: ## stop the stack and drop volumes
	docker compose down -v

## logs: tail logs for all services
logs: ## tail logs for all services
	docker compose logs -f --tail=100

## ps: list running services
ps: ## list running services
	docker compose ps

## proto: regenerate Go and Python gRPC stubs
proto: ## regenerate Go and Python gRPC stubs (requires protoc + plugins)
	@command -v protoc >/dev/null || { echo "protoc not found; brew install protobuf"; exit 1; }
	@command -v protoc-gen-go >/dev/null || go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@command -v protoc-gen-go-grpc >/dev/null || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@python3 -c "import grpc_tools" 2>/dev/null || python3 -m pip install --user grpcio-tools
	mkdir -p orchestrator/gen/optimizerpb optimizer/gen
	PATH="$$PATH:$$(go env GOPATH)/bin" protoc -I proto \
		--go_out=orchestrator/gen/optimizerpb --go_opt=paths=source_relative \
		--go-grpc_out=orchestrator/gen/optimizerpb --go-grpc_opt=paths=source_relative \
		proto/optimizer.proto
	python3 -m grpc_tools.protoc -I proto \
		--python_out=optimizer/gen --grpc_python_out=optimizer/gen --pyi_out=optimizer/gen \
		proto/optimizer.proto
	touch optimizer/gen/__init__.py

## build: build all go binaries locally (sanity check)
build: ## build all go binaries locally (sanity check)
	cd producer && go build ./...
	cd orchestrator && go build ./...

## demo: wait for traffic, then trigger the full pipeline and print results
demo: ## wait ~30s of traffic, then hit the orchestrator API
	./scripts/demo.sh

.PHONY: help up down logs ps proto build demo
