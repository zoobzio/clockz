# Testing Infrastructure

Comprehensive testing suite for clockz, covering performance characteristics and integration scenarios.

## Structure

### benchmarks/
Performance benchmarks measuring real-world concerns:
- **Scaling**: Performance with varying numbers of concurrent waiters (10-2000)
- **Concurrency**: Parallel access patterns and mutex contention
- **Memory**: Allocation patterns and leak detection
- **Context**: Parent-child context cancellation overhead

Key findings:
- Linear performance up to ~1000 waiters
- Stable memory usage (~108KB growth in extended cycles)
- Primary cost is slice operations during waiter management

### integration/
Integration tests validating core functionality:
- **Determinism**: Proves timer firing order and pipeline synchronization
- **Parent Cancellation**: Validates context cancellation propagation

## Running Tests

```bash
# Run all benchmarks
cd benchmarks
go test -bench=. -benchmem

# Run specific benchmark suite
go test -bench=BenchmarkScaling -benchmem

# Run integration tests
cd ../integration
go test -v
```

## Benchmark Interpretation

The benchmarks simulate realistic test suite workloads:
- Mixed timer/ticker/context operations
- Burst creation followed by cleanup
- Progressive scaling to identify performance boundaries

Results show clockz handles typical test suite loads (hundreds of timers) efficiently, with predictable degradation at extreme scales.