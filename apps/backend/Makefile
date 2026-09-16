.PHONY: help build run test test-integration lint fmt tidy docker-up docker-down

help: ## Display available commands
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build server binary
	go build -o bin/server ./cmd/server

run: ## Run server locally
	go run ./cmd/server

test: ## Run unit tests
	go test -race -v ./...

test-integration: ## Run integration tests
	go test -tags=integration -race -v ./tests/integration/...

lint: ## Run static analysis checks
	go vet ./...

fmt: ## Format source files
	gofmt -s -w .

tidy: ## Tidy and verify Go module dependencies
	go mod tidy
	go mod verify

docker-up: ## Start local docker environment (Postgres, Redis, App)
	docker compose up -d

docker-down: ## Stop local docker environment
	docker compose down
