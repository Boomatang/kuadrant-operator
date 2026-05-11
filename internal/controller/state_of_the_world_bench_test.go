//go:build prof

/*
Benchmark test harness for the GetKuadrantFromTopology function in state_of_the_world.go.

This file contains benchmark tests to measure the performance of the GetKuadrantFromTopology
function against real cluster topologies.

## Running the benchmarks:

### Real Cluster Benchmarks (Requires Cluster Access or Snapshot)

Run all benchmarks:
  go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology -run=^$ ./internal/controller/ -benchmem

Using snapshot (recommended):
  SNAPSHOTS_DIR=snapshots go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology_RealCluster -run=^$ ./internal/controller/ -benchmem

Using single snapshot file:
  SNAPSHOTS_DIR=snapshots/baseline.yaml go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology_RealCluster -run=^$ ./internal/controller/ -benchmem

### Performance Analysis

Generate CPU profile:
  SNAPSHOTS_DIR=snapshots go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology_RealCluster -run=^$ ./internal/controller/ -cpuprofile=cpu.prof

Generate memory profile:
  SNAPSHOTS_DIR=snapshots go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology_RealCluster -run=^$ ./internal/controller/ -memprofile=mem.prof

Analyze profiles:
  go tool pprof -http=:8080 cpu.prof
  go tool pprof -http=:8080 -alloc_space mem.prof

## Benchmark scenarios:

- BenchmarkGetKuadrantFromTopology_RealCluster: Test with real cluster topology from snapshots
*/

package controllers

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// BenchmarkGetKuadrantFromTopology_RealCluster benchmarks with real cluster topology from snapshots directory
// Loads all snapshot files from the specified directory and runs sub-benchmarks for each.
//
// Usage:
//
//	SNAPSHOTS_DIR=snapshots go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology_RealCluster -run=^$ ./internal/controller/ -benchmem
//
// If SNAPSHOTS_DIR is not set, defaults to "snapshots" in the current directory.
// To benchmark a single snapshot:
//
//	SNAPSHOTS_DIR=snapshots/baseline.yaml go test -tags=prof -bench=BenchmarkGetKuadrantFromTopology_RealCluster -run=^$ ./internal/controller/ -benchmem
func BenchmarkGetKuadrantFromTopology_RealCluster(b *testing.B) {
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
			benchmarkGetKuadrantFromTopology(b, snapshotFile)
		})
	}
}

// benchmarkGetKuadrantFromTopology benchmarks the GetKuadrantFromTopology function with a snapshot
func benchmarkGetKuadrantFromTopology(b *testing.B, snapshotPath string) {
	loader, err := NewClusterTopologyLoader()
	if err != nil {
		b.Fatalf("Failed to create topology loader: %v", err)
	}

	topology, _, err := loader.LoadFromSnapshot(snapshotPath)
	if err != nil {
		b.Fatalf("Failed to load snapshot: %v", err)
	}

	// Verify that there is at least one Kuadrant in the topology
	kuadrant := GetKuadrantFromTopology(topology, nil)
	if kuadrant == nil {
		b.Skip("No Kuadrant instance found in topology")
		return
	}

	state := &sync.Map{}

	b.ResetTimer()

	// Benchmark the function
	for i := 0; i < b.N; i++ {
		_ = GetKuadrantFromTopology(topology, state)
	}
}
