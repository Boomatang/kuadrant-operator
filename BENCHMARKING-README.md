# enforcedCondition Performance Benchmarking

Complete guide to benchmarking the `enforcedCondition` function performance using real cluster topologies.

## Quick Start

```bash
# 1. Create snapshots from your cluster
./bin/snapshot-topology -output=snapshots/baseline.yaml

# 2. Run benchmarks (use absolute path or path relative to project root)
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem

# 3. Compare results with benchstat
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```

## Components

### 1. Snapshot Tool (`bin/snapshot-topology`)

Exports your cluster topology to a YAML file for repeatable benchmarking.

**Build:**
```bash
go build -tags=prof -o bin/snapshot-topology ./cmd/snapshot-topology
```

**Usage:**
```bash
# Cluster snapshot
./bin/snapshot-topology -output=snapshots/full-cluster.yaml

# Custom kubeconfig
./bin/snapshot-topology -config=/path/to/kubeconfig -output=snapshots/cluster.yaml
```

**What it captures:**
- Kuadrant instances
- GatewayClasses
- Gateways
- HTTPRoutes and GRPCRoutes
- AuthPolicies and RateLimitPolicies
- AuthConfigs
- Authorino instances
- EnvoyFilters (Istio)

### 2. Benchmark Tests

Automatically loads all `.yaml` files from `SNAPSHOTS_DIR` and creates sub-benchmarks using `b.Run`.

**`BenchmarkEnforcedCondition_RealCluster`**
- Tests all policies in each snapshot
- Shows total reconciliation time
- Output: `BenchmarkEnforcedCondition_RealCluster/<snapshot-name>`

**`BenchmarkEnforcedCondition_RealClusterSinglePolicy`**
- Tests a single policy from each snapshot
- More consistent results for comparison
- Output: `BenchmarkEnforcedCondition_RealClusterSinglePolicy/<snapshot-name>`

**Usage:**
```bash
# Run all benchmarks on all snapshots
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem

# Run only single-policy benchmarks
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy ./internal/controller/ -benchmem

# Benchmark specific snapshot
SNAPSHOTS_DIR=$PWD/snapshots/baseline.yaml go test -tags=prof -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem

# With custom benchmark time
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem -benchtime=10s

# Save results to file
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem | tee results.txt
```

## Workflow Examples

### Baseline Performance Testing

Establish current performance:

```bash
# 1. Snapshot current state
./bin/snapshot-topology -output=snapshots/baseline-$(date +%Y%m%d).yaml

# 2. Run comprehensive benchmark with profiling
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy \
  ./internal/controller/ -benchmem -benchtime=10s \
  -cpuprofile=cpu.prof -memprofile=mem.prof | tee baseline.txt

# 3. Analyze profiles
go tool pprof -http=:8080 cpu.prof
```

### Performance Over Time

Track performance as cluster grows:

```bash
# Week 1: 150 policies
./bin/snapshot-topology -output=snapshots/week1-150policies.yaml
SNAPSHOTS_DIR=$PWD/snapshots/week1-150policies.yaml go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem | tee week1.txt

# Week 2: 300 policies
./bin/snapshot-topology -output=snapshots/week2-300policies.yaml
SNAPSHOTS_DIR=$PWD/snapshots/week2-300policies.yaml go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem | tee week2.txt

# Week 3: 500 policies
./bin/snapshot-topology -output=snapshots/week3-500policies.yaml
SNAPSHOTS_DIR=$PWD/snapshots/week3-500policies.yaml go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ -benchmem | tee week3.txt

# Compare with benchstat
benchstat week1.txt week3.txt
```

### Finding Performance Bottlenecks

Detailed profiling workflow:

```bash
# 1. Create snapshot
./bin/snapshot-topology -output=snapshots/current.yaml

# 2. Run with CPU profiling
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy \
  ./internal/controller/ -benchmem \
  -cpuprofile=cpu.prof

# 3. Analyze CPU profile
go tool pprof -http=:8080 cpu.prof
# Look for:
#   - Red/orange = high CPU usage
#   - Focus on internal functions (not stdlib)
#   - topology.Objects().Children() calls
#   - Repeated allocations

# 4. Run with memory profiling
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy \
  ./internal/controller/ -benchmem \
  -memprofile=mem.prof

# 5. Analyze memory allocations
go tool pprof -http=:8080 -alloc_space mem.prof
```

### Before/After Optimization

Compare performance before and after code changes:

```bash
# 1. Baseline before optimization
git checkout main
./bin/snapshot-topology -output=snapshots/baseline.yaml
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=10s | tee before.txt

# 2. After optimization
git checkout feature/optimize-enforced-condition
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=10s | tee after.txt

# 3. Statistical comparison
benchstat before.txt after.txt
```

### CI/CD Integration

Add performance regression testing:

```bash
# In CI pipeline:
# 1. Create standard test snapshot (committed to repo)
./bin/snapshot-topology -output=snapshots/ci-baseline.yaml

# 2. Run benchmark in CI
SNAPSHOTS_DIR=$PWD/snapshots/ci-baseline.yaml go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem | tee current.txt

# 3. Compare against baseline (committed baseline.txt in repo)
benchstat baseline.txt current.txt

# 4. Fail if performance degrades >10%
```

## Understanding Results

### Performance Metrics

```
Snapshot: full-cluster
Time/Op:     95.4 ms    - Time per policy check
Allocs/Op:   1.4M       - Memory allocations per policy
Bytes/Op:    70.4 MB    - Memory allocated per policy
Policies:    150        - Total policies in snapshot
```

**What to look for:**
- **Time/Op**: Should be <100ms for production
- **Allocs/Op**: High count (>1M) indicates excessive object creation
- **Bytes/Op**: Shows memory pressure; watch for GC impact
- **Linear scaling**: Time should grow proportionally with policy count

### Throughput Calculation

```
Throughput = 1 / Time per policy
Example: 1 / 0.095s = ~10.5 policies/second
```

For 150 policies: 150 / 10.5 = ~14 seconds total reconciliation time

### Comparing Results

Use `benchstat` for statistical comparison:

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Compare two runs
benchstat old.bench.txt new.bench.txt
```

**Output:**
```
name                                   old time/op    new time/op    delta
EnforcedCondition_RealClusterSingle-16   95.5ms ± 2%    45.2ms ± 1%  -52.67%  (p=0.000)

name                                   old alloc/op   new alloc/op   delta
EnforcedCondition_RealClusterSingle-16   70.4MB ± 0%    35.1MB ± 0%  -50.14%  (p=0.000)

name                                   old allocs/op  new allocs/op  delta
EnforcedCondition_RealClusterSingle-16   1.40M ± 0%     0.70M ± 0%  -50.00%  (p=0.000)
```

- **Positive delta** = slower (regression)
- **Negative delta** = faster (improvement)
- **p-value < 0.05** = statistically significant

## Profiling Analysis

### CPU Profile

Shows where time is spent:

```bash
# Run benchmark with CPU profiling
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy \
  ./internal/controller/ -benchmem \
  -cpuprofile=cpu.prof

# Interactive web UI
go tool pprof -http=:8080 cpu.prof

# Command line (top 10 functions)
go tool pprof -top cpu.prof
```

**Look for:**
- Functions consuming >5% CPU
- Internal functions (not stdlib/runtime)
- Repeated calls to expensive operations
- Topology traversal costs

### Memory Profile

Shows allocation hot spots:

```bash
# Run benchmark with memory profiling
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy \
  ./internal/controller/ -benchmem \
  -memprofile=mem.prof

# Interactive web UI (allocated space)
go tool pprof -http=:8080 -alloc_space mem.prof

# Command line (top allocations)
go tool pprof -top -alloc_space mem.prof
```

**Look for:**
- Functions allocating >10% of total memory
- Repeated small allocations (can be optimized with pooling)
- Large temporary objects
- Map/slice growth

## Performance Targets

Based on production requirements:

| Metric | Target | Warning | Critical |
|--------|--------|---------|----------|
| Time per policy | <50ms | 50-100ms | >100ms |
| Allocations per policy | <500K | 500K-1M | >1M |
| Memory per policy | <10MB | 10-50MB | >50MB |
| 100 policy reconciliation | <5s | 5-10s | >10s |

## Troubleshooting

### "No Kuadrant instance found"

Verify that your cluster has Kuadrant resources:

```bash
# Create cluster snapshot
./bin/snapshot-topology -output=snapshots/full-cluster.yaml

# Verify Kuadrant is included
grep '"kind":"Kuadrant"' snapshots/full-cluster.yaml
```

### Benchmarks Take Too Long

Reduce benchmark time for faster iteration:

```bash
# Run only 1 iteration per snapshot
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=1x

# Or shorter time duration
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=1s
```

### Inconsistent Results

Use more iterations for stable results:

```bash
# Run longer benchmark
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=30s | tee run1.txt

# Compare multiple runs
SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=10s | tee run2.txt

SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
  -bench=BenchmarkEnforcedCondition ./internal/controller/ \
  -benchmem -benchtime=10s | tee run3.txt

# Use benchstat to see variance
benchstat run1.txt run2.txt run3.txt
```

## Best Practices

1. **Consistent snapshots**: Use same cluster state for comparisons
2. **Stable environment**: Run on idle system (close other apps)
3. **Multiple runs**: Run benchmarks 3x and average results
4. **Version snapshots**: Name snapshots with date/policy-count
5. **Commit baselines**: Check in reference snapshots to git
6. **Profile regularly**: Generate profiles weekly to catch regressions
7. **Track trends**: Keep historical results to see long-term trends

## Files in This Directory

```
├── bin/
│   └── snapshot-topology         # Snapshot capture tool
├── snapshots/
│   ├── full-cluster.yaml         # Your snapshots here
│   └── baseline-*.yaml
├── cmd/snapshot-topology/        # Snapshot tool source
├── internal/controller/
│   ├── auth_policy_status_updater_bench_test.go  # Benchmark tests
│   └── cluster_topology_loader.go                # Topology loader
└── BENCHMARKING-README.md       # This file
```

## Next Steps

1. **Create your first snapshot**:
   ```bash
   ./bin/snapshot-topology -output=snapshots/baseline.yaml
   ```

2. **Run benchmarks**:
   ```bash
   SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
     -bench=BenchmarkEnforcedCondition ./internal/controller/ \
     -benchmem | tee baseline.txt
   ```

3. **Profile if needed**:
   ```bash
   SNAPSHOTS_DIR=$PWD/snapshots go test -tags=prof \
     -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy \
     ./internal/controller/ -benchmem \
     -cpuprofile=cpu.prof -memprofile=mem.prof

   go tool pprof -http=:8080 cpu.prof
   ```

4. **Track over time**: Create new snapshots as you scale and compare results with benchstat!
