.PHONY: test bench bench-all lint coverage clean all help test-integration test-all ci

# Default target
all: test lint

# Display help
help:
	@echo "clockz Development Commands"
	@echo "========================="
	@echo ""
	@echo "Testing & Quality:"
	@echo "  make test            - Run unit tests with race detector"
	@echo "  make test-integration- Run integration tests with race detector"
	@echo "  make test-all        - Run all test suites (unit + integration)"
	@echo "  make bench           - Run core library benchmarks"
	@echo "  make bench-all       - Run all benchmarks (core + integration)"
	@echo "  make lint            - Run linters"
	@echo "  make lint-fix        - Run linters with auto-fix"
	@echo "  make coverage        - Generate coverage report (HTML)"
	@echo "  make check           - Run tests and lint (quick check)"
	@echo "  make ci              - Full CI simulation (all tests + quality checks)"
	@echo ""
	@echo "Other:"
	@echo "  make install-tools- Install required development tools"
	@echo "  make clean        - Clean generated files"
	@echo "  make all          - Run tests and lint (default)"

# Run tests with race detector
test:
	@echo "Running core tests..."
	@go test -v -race ./...

# Run core benchmarks
bench:
	@echo "Running core benchmarks..."
	@go test -bench=. -benchmem -benchtime=100ms -timeout=15m .

# Run all benchmarks including integration
bench-all:
	@echo "Running all benchmarks..."
	@echo "=== Core Library Benchmarks ==="
	@go test -bench=. -benchmem -benchtime=100ms -timeout=15m ./...

# Run linters
lint:
	@echo "Running linters..."
	@golangci-lint run --config=.golangci.yml --timeout=5m

# Run linters with auto-fix
lint-fix:
	@echo "Running linters with auto-fix..."
	@golangci-lint run --config=.golangci.yml --fix

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out $$(go list ./... | grep -v '/testing/')
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1
	@echo "Coverage report generated: coverage.html"

# Clean generated files
clean:
	@echo "Cleaning..."
	@rm -f coverage.out coverage.html
	@find . -name "*.test" -delete
	@find . -name "*.prof" -delete
	@find . -name "*.out" -delete

# Install development tools
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8

# Quick check - run tests and lint
check: test lint
	@echo "All checks passed!"

# Integration tests - component interaction verification
test-integration:
	@echo "Running integration tests..."
	@go test -v -race -timeout=10m ./testing/integration/...

# Comprehensive test suite - all tests with race detection
test-all: test test-integration
	@echo "All test suites completed!"

# CI simulation - what CI runs locally
ci: clean lint test test-integration bench coverage
	@echo "Full CI simulation complete!"