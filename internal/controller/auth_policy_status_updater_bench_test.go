//go:build prof

/*
Benchmark test harness for the enforcedCondition function in AuthPolicyStatusUpdater.

This file contains benchmark tests to measure the performance of the enforcedCondition
function against real cluster topologies.

## Running the benchmarks:

### Real Cluster Benchmarks (Requires Cluster Access or Snapshot)

Run all benchmarks:
  go test -tags=prof -bench=BenchmarkEnforcedCondition -run=^$ ./internal/controller/ -benchmem

Using snapshot (recommended):
  SNAPSHOT_PATH=cluster-snapshot.yaml go test -tags=prof -bench=BenchmarkEnforcedCondition_RealCluster -run=^$ ./internal/controller/ -benchmem

Using live cluster:
  KUBECONFIG=~/.kube/config go test -tags=prof -bench=BenchmarkEnforcedCondition_RealCluster -run=^$ ./internal/controller/ -benchmem

Benchmark specific policy:
  SNAPSHOT_PATH=cluster-snapshot.yaml POLICY_NAME=my-policy go test -tags=prof -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy -run=^$ ./internal/controller/ -benchmem

### Performance Analysis

Generate CPU profile:
  SNAPSHOT_PATH=cluster-snapshot.yaml go test -tags=prof -bench=BenchmarkEnforcedCondition_RealCluster -run=^$ ./internal/controller/ -cpuprofile=cpu.prof

Generate memory profile:
  SNAPSHOT_PATH=cluster-snapshot.yaml go test -tags=prof -bench=BenchmarkEnforcedCondition_RealCluster -run=^$ ./internal/controller/ -memprofile=mem.prof

Analyze profiles:
  go tool pprof -http=:8080 cpu.prof
  go tool pprof -http=:8080 -alloc_space mem.prof

## Benchmark scenarios:

- BenchmarkEnforcedCondition_RealCluster: All policies from real cluster topology
- BenchmarkEnforcedCondition_RealClusterSinglePolicy: Single policy from real cluster (for detailed profiling)

## Creating Cluster Snapshots

Build snapshot tool:
  go build -tags=prof -o bin/snapshot-topology ./cmd/snapshot-topology

Capture cluster:
  ./bin/snapshot-topology -output=cluster-snapshot.yaml

See BENCHMARKING-README.md for detailed instructions.
*/

package controllers

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-logr/logr"

	kuadrantv1 "github.com/kuadrant/kuadrant-operator/api/v1"
)

// BenchmarkEnforcedCondition_RealCluster benchmarks with real cluster topology from snapshots directory
// Loads all snapshot files from the specified directory and runs sub-benchmarks for each.
//
// Usage:
//
//	SNAPSHOTS_DIR=snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition_RealCluster -run=^$ ./internal/controller/ -benchmem
//
// If SNAPSHOTS_DIR is not set, defaults to "snapshots" in the current directory.
// To benchmark a single snapshot:
//
//	SNAPSHOTS_DIR=snapshots/baseline.yaml go test -tags=prof -bench=BenchmarkEnforcedCondition_RealCluster -run=^$ ./internal/controller/ -benchmem
func BenchmarkEnforcedCondition_RealCluster(b *testing.B) {
	snapshotsPath := os.Getenv("SNAPSHOTS_DIR")
	if snapshotsPath == "" {
		snapshotsPath = "snapshots"
	}

	// Check if path is a file or directory
	info, err := os.Stat(snapshotsPath)
	if err != nil {
		b.Skipf("Snapshots path not found: %s (set SNAPSHOTS_DIR env var)", snapshotsPath)
		return
	}

	var snapshotFiles []string
	if info.IsDir() {
		// Read all YAML files from directory
		entries, err := os.ReadDir(snapshotsPath)
		if err != nil {
			b.Fatalf("Failed to read snapshots directory: %v", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() && (filepath.Ext(entry.Name()) == ".yaml" || filepath.Ext(entry.Name()) == ".yml") {
				snapshotFiles = append(snapshotFiles, filepath.Join(snapshotsPath, entry.Name()))
			}
		}
	} else {
		// Single file
		snapshotFiles = []string{snapshotsPath}
	}

	if len(snapshotFiles) == 0 {
		b.Skipf("No snapshot files found in: %s", snapshotsPath)
		return
	}

	b.Logf("Found %d snapshot file(s)", len(snapshotFiles))

	// Run sub-benchmark for each snapshot
	for _, snapshotFile := range snapshotFiles {
		snapshotName := filepath.Base(snapshotFile)
		snapshotName = snapshotName[:len(snapshotName)-len(filepath.Ext(snapshotName))] // Remove extension

		b.Run(snapshotName, func(b *testing.B) {
			benchmarkSnapshotAllPolicies(b, snapshotFile)
		})
	}
}

// benchmarkSnapshotAllPolicies benchmarks all policies in a snapshot
func benchmarkSnapshotAllPolicies(b *testing.B, snapshotPath string) {
	loader, err := NewClusterTopologyLoader()
	if err != nil {
		b.Fatalf("Failed to create topology loader: %v", err)
	}

	topology, _, err := loader.LoadFromSnapshot(snapshotPath)
	if err != nil {
		b.Fatalf("Failed to load snapshot: %v", err)
	}

	state := &sync.Map{}

	// Find a Kuadrant instance
	kuadrant := GetKuadrantFromTopology(topology, state)
	if kuadrant == nil {
		b.Skip("No Kuadrant instance found in topology")
		return
	}

	// Calculate effective policies
	effectivePolicies := CalculateEffectiveAuthPolicies(context.TODO(), topology, kuadrant, &sync.Map{})

	// Create state
	state.Store(StateEffectiveAuthPolicies, effectivePolicies)
	state.Store(StateAuthPolicyValid, map[string]error{})

	// Find all AuthPolicies in the topology
	authPolicies := make([]*kuadrantv1.AuthPolicy, 0)
	for _, policy := range topology.Policies().Items() {
		if ap, ok := policy.(*kuadrantv1.AuthPolicy); ok {
			authPolicies = append(authPolicies, ap)
		}
	}

	if len(authPolicies) == 0 {
		b.Skip("No AuthPolicies found in topology")
		return
	}

	// b.Logf("Loaded topology with %d AuthPolicies, %d effective policies",
	// 	len(authPolicies), len(effectivePolicies))

	logger := logr.Discard()

	b.ResetTimer()

	// Benchmark each policy
	for i := 0; i < b.N; i++ {
		for _, policy := range authPolicies {
			updater := &AuthPolicyStatusUpdater{}
			_ = updater.enforcedCondition(policy, topology, state, logger)
		}
	}
}

// BenchmarkEnforcedCondition_RealClusterSinglePolicy benchmarks a single policy from snapshots directory
// Loads all snapshot files and runs sub-benchmarks for the first policy in each snapshot.
// Useful for detailed profiling and consistent single-policy measurements.
//
// Usage:
//
//	SNAPSHOTS_DIR=snapshots go test -tags=prof -bench=BenchmarkEnforcedCondition_RealClusterSinglePolicy -run=^$ ./internal/controller/ -benchmem
//
// If SNAPSHOTS_DIR is not set, defaults to "snapshots" in the current directory.
func BenchmarkEnforcedCondition_RealClusterSinglePolicy(b *testing.B) {
	snapshotsPath := os.Getenv("SNAPSHOTS_DIR")
	if snapshotsPath == "" {
		snapshotsPath = "snapshots"
	}

	// Check if path is a file or directory
	info, err := os.Stat(snapshotsPath)
	if err != nil {
		b.Skipf("Snapshots path not found: %s (set SNAPSHOTS_DIR env var)", snapshotsPath)
		return
	}

	var snapshotFiles []string
	if info.IsDir() {
		// Read all YAML files from directory
		entries, err := os.ReadDir(snapshotsPath)
		if err != nil {
			b.Fatalf("Failed to read snapshots directory: %v", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() && (filepath.Ext(entry.Name()) == ".yaml" || filepath.Ext(entry.Name()) == ".yml") {
				snapshotFiles = append(snapshotFiles, filepath.Join(snapshotsPath, entry.Name()))
			}
		}
	} else {
		// Single file
		snapshotFiles = []string{snapshotsPath}
	}

	if len(snapshotFiles) == 0 {
		b.Skipf("No snapshot files found in: %s", snapshotsPath)
		return
	}

	b.Logf("Found %d snapshot file(s)", len(snapshotFiles))

	// Run sub-benchmark for each snapshot
	for _, snapshotFile := range snapshotFiles {
		snapshotName := filepath.Base(snapshotFile)
		snapshotName = snapshotName[:len(snapshotName)-len(filepath.Ext(snapshotName))] // Remove extension

		b.Run(snapshotName, func(b *testing.B) {
			benchmarkSnapshotSinglePolicy(b, snapshotFile)
		})
	}
}

// benchmarkSnapshotSinglePolicy benchmarks a single policy from a snapshot
func benchmarkSnapshotSinglePolicy(b *testing.B, snapshotPath string) {
	loader, err := NewClusterTopologyLoader()
	if err != nil {
		b.Fatalf("Failed to create topology loader: %v", err)
	}

	topology, _, err := loader.LoadFromSnapshot(snapshotPath)
	if err != nil {
		b.Fatalf("Failed to load snapshot: %v", err)
	}

	state := &sync.Map{}
	kuadrant := GetKuadrantFromTopology(topology, state)
	if kuadrant == nil {
		b.Skip("No Kuadrant instance found in topology")
		return
	}

	effectivePolicies := CalculateEffectiveAuthPolicies(context.TODO(), topology, kuadrant, &sync.Map{})

	state.Store(StateEffectiveAuthPolicies, effectivePolicies)
	state.Store(StateAuthPolicyValid, map[string]error{})

	// Find first AuthPolicy
	var targetPolicy *kuadrantv1.AuthPolicy
	for _, policy := range topology.Policies().Items() {
		if ap, ok := policy.(*kuadrantv1.AuthPolicy); ok {
			targetPolicy = ap
			break
		}
	}

	if targetPolicy == nil {
		b.Skip("No AuthPolicy found in topology")
		return
	}

	// b.Logf("Benchmarking policy: %s/%s", targetPolicy.Namespace, targetPolicy.Name)

	logger := logr.Discard()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		updater := &AuthPolicyStatusUpdater{}
		_ = updater.enforcedCondition(targetPolicy, topology, state, logger)
	}
}
