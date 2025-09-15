# Changelog

All notable changes to this project will be documented in this file.

## [v0.0.1] - 2025-01-15

### 🎉 Initial Release
First release of clockz - deterministic time control for Go testing, extracted from the streamz package to serve as a foundational timing library for the AEGIS framework.

### ✨ Features
- **Clock Interface**: Comprehensive abstraction over Go's time package with 9 core methods
- **RealClock**: Production implementation delegating to standard library
- **FakeClock**: Deterministic time control for testing with manual time advancement
- **Sleep() & Since()**: Additional convenience methods for common time operations
- **Context Integration**: `WithTimeout()` and `WithDeadline()` support for context-aware timing
- **BlockUntilReady()**: Synchronization primitive ensuring deterministic timer delivery
- **Parent Context Propagation**: Proper cancellation inheritance in timeout contexts
- **Thread-Safe Operations**: Full mutex protection for concurrent usage

### 🧪 Testing
- **90% Test Coverage**: Comprehensive unit and integration tests
- **Race Condition Testing**: 100-iteration stress tests for timer delivery
- **Integration Patterns**: Real-world examples (rate limiting, circuit breakers, streaming windows)
- **Parent Cancellation Tests**: Extensive validation of context cancellation propagation
- **Benchmark Suite**: Performance testing for scaling, concurrency, and memory patterns

### 📚 Documentation
- Complete API documentation with interface specifications
- Migration guide from time.Sleep to deterministic testing
- Practical examples solving flaky test issues
- Benchmark analysis documenting O(n log n) waiter sorting behavior

### 🔧 Infrastructure
- **AEGIS Compliance**: Full governance structure (SECURITY.md, CONTRIBUTING.md, LICENSE)
- **CI/CD Pipeline**: GitHub Actions with test, lint, and coverage workflows
- **Release Automation**: GoReleaser configuration for library distribution
- **Code Quality**: golangci-lint with strict configuration
- **Security Scanning**: CodeQL analysis enabled
- **Coverage Reporting**: Codecov integration

### 🎯 Key Problems Solved
- Eliminates flaky tests caused by time.Sleep() and real time operations
- Provides deterministic testing for timeout-dependent code
- Enables instant test execution without actual time delays
- Supports complex timing scenarios in streaming and pipeline architectures

---

For future releases, this changelog will be automatically generated from commit messages following conventional commits specification.