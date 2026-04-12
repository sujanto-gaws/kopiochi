.PHONY: help build run test clean docker-build docker-run generate \
        lint fmt tidy init-project install-hooks

# Variables
BINARY_NAME?=kopiochi
GO?=go
GOFMT?=gofmt
DOCKER_IMAGE?=kopiochi
DB_HOST?=localhost
DB_PORT?=5432
DB_USER?=postgres
DB_PASSWORD?=postgres
DB_NAME?=kopiochi

# Default target
help: ## Show this help message
	@echo "Kopiochi - DDD Go Web API Boilerplate"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""

# Build
build: ## Build the application binary
	@echo "Building $(BINARY_NAME)..."
	$(GO) build -o bin/$(BINARY_NAME) ./cmd/api

# Run
run: ## Run the application server
	@echo "Starting server..."
	$(GO) run ./cmd/api serve

# Run with custom config
run-config: ## Run with custom config file (usage: make run-config CONFIG=config/production.yaml)
	@echo "Starting server with $(CONFIG)..."
	$(GO) run ./cmd/api serve --config $(CONFIG)

# Test
test: ## Run all tests
	@echo "Running tests..."
	$(GO) test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-verbose: ## Run tests with verbose output
	@echo "Running tests (verbose)..."
	$(GO) test -v -cover ./...

# Code quality
lint: ## Run go lint (if golangci-lint installed)
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

fmt: ## Format Go source files
	@echo "Formatting code..."
	$(GOFMT) -s -w .

tidy: ## Clean up Go module dependencies
	@echo "Tidying Go modules..."
	$(GO) mod tidy

vet: ## Run go vet
	@echo "Running go vet..."
	$(GO) vet ./...

check: fmt vet tidy ## Run all code quality checks

# Database
db-migrate: ## Run database migrations (placeholder)
	@echo "Running migrations..."
	@echo "TODO: Implement migration runner"

db-seed: ## Seed the database with test data (placeholder)
	@echo "Seeding database..."
	@echo "TODO: Implement seeder"

# Code generation
generate: ## Generate new CRUD domain (usage: make generate DOMAIN=Product FIELDS="name:string,price:float64")
	@if [ -z "$(DOMAIN)" ]; then \
		echo "Error: DOMAIN is required. Usage: make generate DOMAIN=Product FIELDS='name:string,price:float64'"; \
		exit 1; \
	fi
	@echo "Generating CRUD for $(DOMAIN)..."
	$(GO) run ./cmd/generator -domain $(DOMAIN) -fields "$(FIELDS)"

generate-with-module: ## Generate with custom module path (usage: make generate-with-module DOMAIN=Product FIELDS="..." MODULE=github.com/user/project)
	@if [ -z "$(DOMAIN)" ] || [ -z "$(MODULE)" ]; then \
		echo "Error: DOMAIN and MODULE are required. Usage: make generate-with-module DOMAIN=Product FIELDS='...' MODULE=github.com/user/project"; \
		exit 1; \
	fi
	@echo "Generating CRUD for $(DOMAIN) with module $(MODULE)..."
	$(GO) run ./cmd/generator -domain $(DOMAIN) -fields "$(FIELDS)" -module $(MODULE)

# Swagger/OpenAPI
swagger-init: ## Initialize swagger annotations
	@echo "Initializing swagger..."
	swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal

swagger-docs: ## Generate swagger documentation
	@echo "Generating swagger docs..."
	swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
	@echo "Swagger docs generated in docs/"

swagger-serve: ## Serve swagger UI locally (requires python3)
	@echo "Serving swagger UI at http://localhost:8080/swagger/index.html"
	@echo "Make sure to run 'make swagger-docs' first"
	@echo "Start the server with: make run"

# Database Migrations (Goose)
migrate-up: ## Run all pending migrations
	@echo "Running migrations up..."
	$(GO) run ./cmd/migrate up --config $(CONFIG)

migrate-down: ## Rollback the most recent migration
	@echo "Rolling back last migration..."
	$(GO) run ./cmd/migrate down --config $(CONFIG)

migrate-status: ## Show migration status
	@echo "Migration status:"
	$(GO) run ./cmd/migrate status --config $(CONFIG)

migrate-reset: ## Rollback all migrations
	@echo "Resetting all migrations..."
	$(GO) run ./cmd/migrate reset --config $(CONFIG)

migrate-create: ## Create a new migration file (usage: make migrate-create NAME=create_products)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required. Usage: make migrate-create NAME=create_products"; \
		exit 1; \
	fi
	@echo "Creating migration: $(NAME)..."
	$(GO) run ./cmd/migrate create $(NAME) --type sql

migrate-install: ## Install goose CLI tool
	@echo "Installing goose CLI..."
	go install github.com/pressly/goose/v3/cmd/goose@latest
	@echo "Goose installed successfully"

# Docker
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 --env-file .env $(DOCKER_IMAGE)

docker-compose-up: ## Start with docker-compose (if docker-compose.yml exists)
	@if [ -f "docker-compose.yml" ]; then \
		docker-compose up -d; \
	else \
		echo "docker-compose.yml not found"; \
	fi

docker-compose-down: ## Stop docker-compose
	@if [ -f "docker-compose.yml" ]; then \
		docker-compose down; \
	else \
		echo "docker-compose.yml not found"; \
	fi

# Project initialization
init-project: ## Initialize as new project (usage: make init-project PROJECT=myapi AUTHOR="John")
	@if [ -z "$(PROJECT)" ]; then \
		echo "Error: PROJECT is required. Usage: make init-project PROJECT=myapi AUTHOR='John'"; \
		exit 1; \
	fi
	@echo "Initializing project $(PROJECT)..."
	@if [ "$$(uname -s)" = "Darwin" ] || [ "$$(uname -s)" = "Linux" ]; then \
		bash ./scripts/init.sh --project-name $(PROJECT) --author "$(AUTHOR)"; \
	else \
		powershell -ExecutionPolicy Bypass -File ./scripts/init.ps1 -ProjectName $(PROJECT) -Author "$(AUTHOR)"; \
	fi

# Git hooks
install-hooks: ## Install git hooks
	@echo "Installing git hooks..."
	@if [ -d ".git" ]; then \
		if [ -d ".githooks" ]; then \
			git config core.hooksPath .githooks; \
			echo "Git hooks installed from .githooks/"; \
		else \
			echo "No .githooks directory found"; \
		fi; \
	else \
		echo "Not a git repository"; \
	fi

# Clean
clean: ## Remove build artifacts
	@echo "Cleaning..."
	@if [ -d "bin" ]; then rm -rf bin; fi
	@if [ -f "coverage.out" ]; then rm -f coverage.out; fi
	@if [ -f "coverage.html" ]; then rm -f coverage.html; fi
	@echo "Clean complete"

# Development shortcuts
dev: build run ## Build and run (shortcut)

watch: ## Auto-rebuild on file changes (requires entr or similar)
	@echo "Watching for changes..."
	@if command -v entr > /dev/null 2>&1; then \
		find . -name "*.go" | entr -r $(GO) run ./cmd/api serve; \
	else \
		echo "entr not installed. Install with: apt-get install entr (Linux) or brew install entr (Mac)"; \
	fi

# CI/CD
ci: check test ## Run CI checks (lint + test)

# Dependencies
deps-update: ## Update Go dependencies
	$(GO) get -u ./...
	$(GO) mod tidy

deps-audit: ## Check for vulnerable dependencies
	@if command -v govulncheck > /dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi
