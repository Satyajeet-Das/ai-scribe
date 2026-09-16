.PHONY: help build-backend run-backend test-backend lint-backend fmt-backend dev-frontend build-frontend docker-up docker-down

help: ## Display available commands
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build-backend: ## Build Go server binary
	cd apps/backend && go build -o bin/server ./cmd/server

run-backend: ## Run Go server locally
	cd apps/backend && go run ./cmd/server

test-backend: ## Run Go unit tests
	cd apps/backend && go test -race -v ./...

lint-backend: ## Run static analysis checks on backend
	cd apps/backend && go vet ./...

fmt-backend: ## Format Go source files
	cd apps/backend && gofmt -s -w .

dev-frontend: ## Run Next.js frontend dev server
	cd apps/frontend && npm run dev

build-frontend: ## Build Next.js frontend
	cd apps/frontend && npm run build

docker-up: ## Start local docker environment (Postgres, Redis, Backend, Frontend)
	docker compose up -d

docker-down: ## Stop local docker environment
	docker compose down
