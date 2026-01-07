.PHONY: test test-unit test-integration test-all bench bench-all lint lint-fix coverage clean all help check ci install-tools install-hooks

.DEFAULT_GOAL := help

# Display help - self-documenting via grep
help: ## Display available commands
	@echo "clockz Development Commands"
	@echo "==========================="
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# Testing targets
test: ## Run unit tests with race detector
	@echo "Running core tests..."
	@go test -v -race ./...

test-unit: ## Run unit tests only (short mode)
	@echo "Running unit tests (short mode)..."
	@go test -v -race -short ./...

test-integration: ## Run integration tests with race detector
	@echo "Running integration tests..."
	@go test -v -race -timeout=10m ./testing/integration/...

test-all: test test-integration ## Run all test suites (unit + integration)
	@echo "All test suites completed!"

# Benchmark targets
bench: ## Run core library benchmarks
	@echo "Running core benchmarks..."
	@go test -bench=. -benchmem -benchtime=100ms -timeout=15m .

bench-all: ## Run all benchmarks (core + integration)
	@echo "Running all benchmarks..."
	@echo "=== Core Library Benchmarks ==="
	@go test -bench=. -benchmem -benchtime=100ms -timeout=15m ./...

test-bench: bench ## Alias for bench (compatibility)

# Code quality targets
lint: ## Run linters
	@echo "Running linters..."
	@golangci-lint run --timeout=5m

lint-fix: ## Run linters with auto-fix
	@echo "Running linters with auto-fix..."
	@golangci-lint run --fix

coverage: ## Generate coverage report (HTML)
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out $$(go list ./... | grep -v '/testing/')
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1
	@echo "Coverage report generated: coverage.html"

# Validation targets
check: test lint ## Quick validation (test + lint)
	@echo "All checks passed!"

ci: clean lint test test-integration bench coverage ## Full CI simulation
	@echo "Full CI simulation complete!"

# Setup targets
install-tools: ## Install required development tools
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6

install-hooks: ## Install git hooks
	@echo "Installing git hooks..."
	@echo '#!/bin/sh' > .git/hooks/pre-commit
	@echo 'make check' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

# Cleanup targets
clean: ## Remove generated files
	@echo "Cleaning..."
	@rm -f coverage.out coverage.html
	@find . -name "*.test" -delete
	@find . -name "*.prof" -delete
	@find . -name "*.out" -delete

# Default target
all: test lint ## Run tests and lint (default)
