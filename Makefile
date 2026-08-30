.PHONY: help build build-release run test clean docker-build docker-run generate \
        lint fmt tidy init-project install-hooks keys \
        arch coverage-check coverage-update size \
        compose-up compose-down compose-reset compose-logs \
        docker-compose-up docker-compose-down \
        swagger-init swagger-docs swagger-run swagger-serve

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
CONFIG?=config/default.yaml
KEYS_DIR?=keys
SWAG_VERSION?=v1.16.4
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/sujanto-gaws/kopiochi/internal/version.Version=$(VERSION)"
RELEASE_LDFLAGS=-ldflags "-s -w -X github.com/sujanto-gaws/kopiochi/internal/version.Version=$(VERSION)"

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
build: ## Build the application binary (development: symbols kept)
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	$(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/api

build-release: ## Build a stripped, trimmed release binary
	@echo "Building $(BINARY_NAME) $(VERSION) (release)..."
	@# -s -w drop the symbol table and DWARF info; -trimpath removes the build
	@# machine's absolute paths from the binary. Panic stack traces survive
	@# all three — only debugger metadata and build-host layout go.
	@#
	@# Kept separate from `build` on purpose: a stripped binary is harder to
	@# attach a debugger to, and that is the wrong default for local work.
	$(GO) build -trimpath $(RELEASE_LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/api

size: build-release ## Report the release binary size
	@ls -lh bin/$(BINARY_NAME) | awk '{print "release binary: " $$5}'

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
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

coverage-check: ## Enforce the per-package coverage floors and the no-regression ratchet
	@echo "Checking coverage policy..."
	@# Known environment limitation, same class as -race: the Go installation
	@# on the machine this repo is maintained from is missing covdata from
	@# pkg/tool, so `go test -coverprofile ./...` exits 1 on the packages that
	@# have no test files. The profile itself is written correctly and the
	@# check below reads it fine. `-` tolerates that exit; CI's toolchain is
	@# complete and the same step there is not tolerant.
	@# No -with-database here: the integration-covered packages report 0%
	@# where no Postgres is reachable, and the tool names them as NOT CHECKED
	@# rather than either failing or silently passing. CI runs it with a
	@# service container and the flag, which is where those floors bite.
	-$(GO) test -p 1 -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) run ./tools/coverage -profile coverage.out

# -p 1 for the reason T7 put it on the CI job: every integration test resolves
# to the same TEST_DATABASE_URL, and package binaries running concurrently
# truncate each other's rows mid-test. The `-` above means make ignores the
# resulting failures, so without this a corrupted run still updates the baseline.
#
# Set TEST_DATABASE_URL before running this if you want the database-backed
# packages recorded. Without one, `-update` skips them rather than writing down
# a number that was never measured — so a laptop run is safe, just incomplete.
# Add -with-database to the second line when a Postgres is reachable.
coverage-update: ## Raise the coverage baseline to match the current run
	-$(GO) test -p 1 -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) run ./tools/coverage -profile coverage.out -update

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

arch: ## Check the dependency rules (docs/architectures/01-modularity/dependency-rules.md)
	@echo "Checking architecture rules..."
	@# -count=1 is required: these tests read the whole repo through
	@# go/packages, but the test cache keys only on the archtest package's
	@# own files, so a violation introduced elsewhere returns a cached PASS.
	$(GO) test -count=1 ./tools/archtest/...

check: fmt vet arch tidy ## Run all code quality checks

# JWT signing keys
keys: ## Generate a fresh RSA keypair into keys/ for JWT signing (refuses to overwrite existing keys)
	@if [ -f "$(KEYS_DIR)/private.pem" ] || [ -f "$(KEYS_DIR)/public.pem" ]; then \
		echo "Error: $(KEYS_DIR)/private.pem or $(KEYS_DIR)/public.pem already exists."; \
		echo "Refusing to overwrite - this would invalidate live tokens signed with the current key."; \
		echo "Remove the existing file(s) manually first if you really intend to rotate the keypair."; \
		exit 1; \
	fi
	@mkdir -p $(KEYS_DIR)
	@echo "Generating RSA keypair in $(KEYS_DIR)/..."
	# -traditional forces PKCS1 ("BEGIN RSA PRIVATE KEY") output, which is
	# what internal/infrastructure/token/jwt.go parses via
	# x509.ParsePKCS1PrivateKey. Modern OpenSSL (3.x) defaults to PKCS8
	# ("BEGIN PRIVATE KEY") instead, which that loader cannot read.
	openssl genrsa -traditional -out $(KEYS_DIR)/private.pem 2048
	openssl rsa -in $(KEYS_DIR)/private.pem -pubout -out $(KEYS_DIR)/public.pem
	@echo "Keypair generated: $(KEYS_DIR)/private.pem, $(KEYS_DIR)/public.pem"

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
swagger-init: swagger-docs ## Deprecated alias for swagger-docs

swagger-docs: ## Generate the swagger spec into docs/ (not committed)
	@echo "Generating swagger docs..."
	@command -v swag >/dev/null 2>&1 || { \
		echo "swag not installed. Run:"; \
		echo "  go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)"; \
		exit 1; \
	}
	swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
	@echo "Generated docs/docs.go, docs/swagger.json, docs/swagger.yaml (all gitignored)."

swagger-run: swagger-docs ## Generate the spec and run the server with the UI mounted
	@# The UI is behind a build tag as of Phase 5.3: it links swaggo/swag, the
	@# embedded Swagger UI distribution and four go-openapi packages, none of
	@# which a production server needs. Leaving it out halves the binary.
	$(GO) run -tags swagger ./cmd/api serve --config $(CONFIG)

swagger-serve: swagger-run ## Deprecated alias for swagger-run

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

docker-run: ## Run Docker container (requires a .env — copy .env.example)
	@if [ ! -f .env ]; then \
		echo "Error: .env not found. Copy .env.example and set APP_DB_PASSWORD."; \
		echo "For a full local stack including Postgres, use 'make compose-up' instead."; \
		exit 1; \
	fi
	docker run --rm -p 127.0.0.1:8080:8080 --env-file .env $(DOCKER_IMAGE)

compose-up: ## Start the local stack (Postgres, migrations, API)
	@# The stack requires APP_DB_PASSWORD; compose fails with an explanation
	@# rather than defaulting to a value Config.Validate would reject anyway.
	docker compose up -d --build

compose-down: ## Stop the local stack (keeps the database volume)
	docker compose down

compose-reset: ## Stop the local stack and delete the database volume
	docker compose down -v

compose-logs: ## Follow the API logs
	docker compose logs -f api

# Kept as aliases: these were the documented names, and the hyphenated
# `docker-compose` binary they used is the deprecated v1 CLI.
docker-compose-up: compose-up ## Deprecated alias for compose-up
docker-compose-down: compose-down ## Deprecated alias for compose-down

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
