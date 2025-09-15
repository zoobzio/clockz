# FakeClock Performance Benchmarks

These benchmarks measure FakeClock implementation behavior under various conditions, providing performance baselines for test suite optimization.

## Benchmark Categories

### 1. Scaling Benchmarks (`scaling_bench_test.go`)
Measures performance characteristics with varying waiter counts:
- **BenchmarkWaiterScaling**: Performance scaling with increasing waiter count
- **BenchmarkAdvanceWithManyTimers**: Test suite advancement patterns
- **BenchmarkTickerScaling**: Ticker rescheduling behavior
- **BenchmarkMixedWaiterTypes**: Timer/ticker/context combination performance

### 2. Context Operations (`context_bench_test.go`)
Measures context-specific overhead and mutex contention:
- **BenchmarkContextCreation**: Context waiter registration cost
- **BenchmarkContextTimeout**: Context processing in setTimeLocked
- **BenchmarkContextCancellation**: removeContextWaiter performance
- **BenchmarkConcurrentContextOps**: Dual mutex contention
- **BenchmarkNestedContexts**: Parent cancellation propagation

### 3. Concurrency Benchmarks (`concurrency_bench_test.go`)
Measures concurrent access patterns:
- **BenchmarkConcurrentTimerCreation**: Concurrent timer creation performance
- **BenchmarkConcurrentAdvancement**: Concurrent Advance() call patterns
- **BenchmarkConcurrentMixedOps**: Mixed operation concurrency
- **BenchmarkRaceConditionStress**: Lock mechanism behavior under stress
- **BenchmarkTimerResetContention**: Concurrent waiter modification

### 4. Memory Stability (`memory_bench_test.go`)
Detects memory leaks and allocation pressure:
- **BenchmarkMemoryLeakDetection**: Sustained operation memory growth
- **BenchmarkAllocationPatterns**: Allocation hotspot identification
- **BenchmarkGCPressure**: Garbage collection impact
- **BenchmarkWaiterSliceGrowth**: Slice management overhead
- **BenchmarkLongRunningStability**: Extended operation stability

## Running the Benchmarks

```bash
# Run all benchmarks with memory profiling
go test -bench=. -benchmem ./testing/benchmarks/

# Focus on scaling issues
go test -bench=BenchmarkWaiterScaling -benchmem ./testing/benchmarks/

# Test concurrency bottlenecks
go test -bench=BenchmarkConcurrentAdvancement -benchmem ./testing/benchmarks/

# Memory leak detection
go test -bench=BenchmarkMemoryLeak -benchmem ./testing/benchmarks/

# Full performance profile
go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./testing/benchmarks/
```

## Expected Results

**Scaling Performance**: Expected performance scaling with waiter count
- Performance scales with waiter count as expected for sorting operations
- Timing varies based on system and waiter complexity

**Memory Patterns**: Allocation behavior from slice operations
- Slice operations during waiter processing
- Context waiter slice management
- Send coordination memory usage

**Concurrency Characteristics**: Lock coordination patterns
- Main mutex coordinates waiter operations
- Context mutex manages context-specific state
- BlockUntilReady coordination overhead

## Benchmark Usage

These benchmarks provide baseline measurements for:
1. **Performance characterization** across different waiter counts
2. **Memory allocation patterns** during typical operations
3. **Concurrency behavior** under parallel access
4. **Context operation performance** for timeout scenarios
5. **Long-term stability** during extended operation

## Benchmark Design Principles

- **Realistic usage patterns**: Based on actual test suite behavior
- **Progressive complexity**: From simple to complex scenarios
- **Resource measurement**: Memory allocations and timing
- **Concurrent stress testing**: Real-world parallel access
- **Long-term stability**: Extended operation leak detection

These benchmarks provide performance baselines for understanding FakeClock behavior in various test suite scenarios.