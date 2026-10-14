# CargoFlow — developer commands. Recipes use POSIX sh (Git Bash on Windows).

SERVICES := gateway auth shipment tracking notification
COMPOSE  := docker compose -f deploy/docker-compose.yml
EXE      := $(if $(filter Windows_NT,$(OS)),.exe,)
# The Ryuk reaper container fails to start on Docker Desktop for Windows; tests clean up
# their containers themselves (testcontainers.CleanupContainer), so it is safe to disable.
RYUK_DISABLED := $(if $(filter Windows_NT,$(OS)),true,false)

# Export variables from .env (if present) to every recipe, e.g. for goose.
-include .env
export

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_%-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ---------- tools ----------
.PHONY: tools
tools: ## Check external tools and build Go tools pinned in go.mod
	@command -v go     >/dev/null || { echo "go is missing: https://go.dev/dl/"; exit 1; }
	@command -v buf    >/dev/null || { echo "buf is missing: scoop install buf | brew install bufbuild/buf/buf"; exit 1; }
	@command -v docker >/dev/null || { echo "docker is missing: https://docs.docker.com/get-docker/"; exit 1; }
	go mod download
	go build tool
	@echo "tools ok: $$(go version), buf $$(buf --version)"

# ---------- code generation ----------
.PHONY: proto
proto: ## Lint proto files and generate Go code + OpenAPI spec
	cd proto && buf lint && buf generate

.PHONY: sqlc
sqlc: ## Generate type-safe DB code for auth, shipment and tracking
	go tool sqlc generate -f services/auth/sqlc.yaml
	go tool sqlc generate -f services/shipment/sqlc.yaml
	go tool sqlc generate -f services/tracking/sqlc.yaml

.PHONY: generate
generate: proto sqlc ## Run all code generators

# ---------- database migrations ----------
.PHONY: migrate-up
migrate-up: ## Apply migrations for auth, shipment and tracking
	go tool goose -dir services/auth/migrations postgres "$(AUTH_DATABASE_URL)" up
	go tool goose -dir services/shipment/migrations postgres "$(SHIPMENT_DATABASE_URL)" up
	go tool goose -dir services/tracking/migrations postgres "$(TRACKING_DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration of auth, shipment and tracking
	go tool goose -dir services/auth/migrations postgres "$(AUTH_DATABASE_URL)" down
	go tool goose -dir services/shipment/migrations postgres "$(SHIPMENT_DATABASE_URL)" down
	go tool goose -dir services/tracking/migrations postgres "$(TRACKING_DATABASE_URL)" down

.PHONY: migrate-status
migrate-status: ## Show migration status
	go tool goose -dir services/auth/migrations postgres "$(AUTH_DATABASE_URL)" status
	go tool goose -dir services/shipment/migrations postgres "$(SHIPMENT_DATABASE_URL)" status
	go tool goose -dir services/tracking/migrations postgres "$(TRACKING_DATABASE_URL)" status

# ---------- local infrastructure ----------
.PHONY: infra-up
infra-up: ## Start Postgres, Redis, NATS, Jaeger and wait until healthy
	$(COMPOSE) up -d --wait

.PHONY: infra-down
infra-down: ## Stop infrastructure (data volumes are kept)
	$(COMPOSE) --profile apps down

.PHONY: infra-reset
infra-reset: ## Stop infrastructure and delete all data volumes
	$(COMPOSE) --profile apps down -v

.PHONY: apps-up
apps-up: ## Build and start all Go services in Docker (profile "apps")
	$(COMPOSE) --profile apps up -d --build

.PHONY: apps-logs
apps-logs: ## Follow logs of the Go services running in Docker
	$(COMPOSE) --profile apps logs -f $(SERVICES)

# ---------- run ----------
.PHONY: run-all
run-all: infra-up migrate-up build ## Start infra, apply migrations, run all services from source (Ctrl+C stops all)
	@echo "stopping Go service containers (if any) to free their ports"
	@$(COMPOSE) --profile apps stop $(SERVICES) >/dev/null 2>&1 || true
	@$(MAKE) --no-print-directory -j$(words $(SERVICES)) $(addprefix start-,$(SERVICES))

start-%:
	./bin/$*$(EXE)

run-%: ## Run one service locally: make run-auth, make run-gateway, ...
	@$(COMPOSE) --profile apps stop $* >/dev/null 2>&1 || true
	go run ./services/$*/cmd

# ---------- quality ----------
.PHONY: test
test: ## Run unit tests
	go test ./...

.PHONY: test-integration
test-integration: ## Run integration tests (testcontainers, needs Docker)
	TESTCONTAINERS_RYUK_DISABLED=$(RYUK_DISABLED) go test -tags integration ./...

.PHONY: lint
lint: ## Run buf lint, go vet and golangci-lint
	cd proto && buf lint
	go vet ./...
	go tool golangci-lint run ./...

# ---------- build ----------
.PHONY: build
build: ## Build all service binaries into ./bin
	@for s in $(SERVICES); do \
		echo "go build $$s"; \
		CGO_ENABLED=0 go build -trimpath -o bin/$$s$(EXE) ./services/$$s/cmd || exit 1; \
	done

.PHONY: docker-build
docker-build: ## Build Docker images for all services
	@for s in $(SERVICES); do \
		docker build -f services/$$s/Dockerfile -t cargoflow-$$s:local . || exit 1; \
	done
