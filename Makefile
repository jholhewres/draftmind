SHELL := /bin/bash
.DEFAULT_GOAL := help

-include .env
export

API := api
WEB := web
BIN := $(API)/bin/draftmind

.PHONY: help
help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

.PHONY: setup
setup: ## Install web dependencies and download Go modules
	cd $(WEB) && pnpm install
	cd $(API) && go mod download

.PHONY: db-up
db-up: ## Start Postgres and wait until it is healthy
	docker compose up -d --wait db

.PHONY: db-down
db-down: ## Stop Postgres (keeps the volume)
	docker compose down

.PHONY: migrate
migrate: ## Apply database migrations
	cd $(API) && go run ./cmd/draftmind migrate

.PHONY: dev
dev: db-up migrate ## Run Postgres, the API and the web dev server
	@trap 'kill 0' EXIT; \
	(cd $(API) && go run ./cmd/draftmind serve) & \
	(cd $(WEB) && pnpm dev) & \
	wait

.PHONY: test
test: test-api test-web ## Run all tests

.PHONY: test-api
test-api: ## Run Go tests
	cd $(API) && go test ./...

.PHONY: test-web
test-web: ## Run web tests
	cd $(WEB) && pnpm test

.PHONY: lint
lint: lint-api lint-web ## Run all linters and type checks

.PHONY: lint-api
lint-api: ## Lint Go code
	cd $(API) && go vet ./... && golangci-lint run ./...

.PHONY: lint-web
lint-web: ## Lint and type-check the web app
	cd $(WEB) && pnpm lint

.PHONY: fmt
fmt: ## Format Go code
	cd $(API) && gofmt -w .

.PHONY: build
build: ## Build the API binary and the web bundle
	cd $(API) && go build -o bin/draftmind ./cmd/draftmind
	cd $(WEB) && pnpm build
