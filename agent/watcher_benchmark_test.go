package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// BenchmarkSingleVsMultipleWatchers compares resource usage between one shared watcher vs multiple watchers
func BenchmarkSingleVsMultipleWatchers(b *testing.B) {
	// Create a temporary directory structure for testing
	testDir, err := os.MkdirTemp("", "watcher_benchmark")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	// Create subdirectories to watch
	numDirs := 10
	dirs := make([]string, numDirs)
	for i := 0; i < numDirs; i++ {
		dir := filepath.Join(testDir, fmt.Sprintf("rule_%d", i))
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatal(err)
		}
		dirs[i] = dir
	}

	b.Run("SingleWatcher", func(b *testing.B) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			b.Fatal(err)
		}
		defer watcher.Close()

		// Add all directories to single watcher
		for _, dir := range dirs {
			if err := watcher.Add(dir); err != nil {
				b.Fatal(err)
			}
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)
		singleWatcherMem := m2.HeapAlloc - m1.HeapAlloc
		b.Logf("Single watcher memory: %d bytes (%d KB)", singleWatcherMem, singleWatcherMem/1024)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate file events by touching files
			for _, dir := range dirs {
				file := filepath.Join(dir, "test.txt")
				if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
					b.Fatal(err)
				}
				os.Remove(file)
			}
		}
	})

	b.Run("MultipleWatchers", func(b *testing.B) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		watchers := make([]*fsnotify.Watcher, numDirs)
		for i := 0; i < numDirs; i++ {
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				b.Fatal(err)
			}
			defer watcher.Close()

			if err := watcher.Add(dirs[i]); err != nil {
				b.Fatal(err)
			}
			watchers[i] = watcher
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)
		multiWatcherMem := m2.HeapAlloc - m1.HeapAlloc
		b.Logf("Multiple watchers memory: %d bytes (%d KB)", multiWatcherMem, multiWatcherMem/1024)
		b.Logf("Per-watcher overhead: %d bytes (%d KB)", multiWatcherMem/uint64(numDirs), (multiWatcherMem/uint64(numDirs))/1024)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate file events by touching files
			for _, dir := range dirs {
				file := filepath.Join(dir, "test.txt")
				if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
					b.Fatal(err)
				}
				os.Remove(file)
			}
		}
	})
}

// BenchmarkWatcherCreation measures the cost of creating new watchers
func BenchmarkWatcherCreation(b *testing.B) {
	testDir, err := os.MkdirTemp("", "watcher_creation")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			b.Fatal(err)
		}
		if err := watcher.Add(testDir); err != nil {
			b.Fatal(err)
		}
		watcher.Close()
	}
}

// BenchmarkWatcherMemoryUsage measures memory usage with varying numbers of watchers
func BenchmarkWatcherMemoryUsage(b *testing.B) {
	testDir, err := os.MkdirTemp("", "watcher_memory")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	watcherCounts := []int{1, 5, 10, 20, 50}

	for _, count := range watcherCounts {
		b.Run(fmt.Sprintf("Watchers_%d", count), func(b *testing.B) {
			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			watchers := make([]*fsnotify.Watcher, count)
			for i := 0; i < count; i++ {
				watcher, err := fsnotify.NewWatcher()
				if err != nil {
					b.Fatal(err)
				}
				defer watcher.Close()

				if err := watcher.Add(testDir); err != nil {
					b.Fatal(err)
				}
				watchers[i] = watcher
			}

			runtime.GC()
			runtime.ReadMemStats(&m2)
			totalMem := m2.HeapAlloc - m1.HeapAlloc
			b.Logf("%d watchers memory: %d bytes (%d KB)", count, totalMem, totalMem/1024)
			if count > 1 {
				b.Logf("Per-watcher overhead: %d bytes (%d KB)", totalMem/uint64(count), (totalMem/uint64(count))/1024)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Let watchers sit idle to measure base overhead
				time.Sleep(1 * time.Microsecond)
			}
		})
	}
}

// TestWatcherResourceUsage provides detailed resource usage analysis
func TestWatcherResourceUsage(t *testing.T) {
	testDir, err := os.MkdirTemp("", "watcher_resource_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	// Create subdirectories
	numDirs := 20
	dirs := make([]string, numDirs)
	for i := 0; i < numDirs; i++ {
		dir := filepath.Join(testDir, fmt.Sprintf("dir_%d", i))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		dirs[i] = dir
	}

	t.Log("=== Watcher Resource Usage Analysis ===")

	// Test single watcher watching all directories
	t.Run("SingleWatcher", func(t *testing.T) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)
		baseline := m1.HeapAlloc

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			t.Fatal(err)
		}
		defer watcher.Close()

		for _, dir := range dirs {
			if err := watcher.Add(dir); err != nil {
				t.Fatal(err)
			}
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)
		current := m2.HeapAlloc

		var mem uint64
		if current > baseline {
			mem = current - baseline
		} else {
			// Handle case where GC freed more than we allocated
			mem = 0
		}

		t.Logf("Single watcher watching %d directories:", numDirs)
		t.Logf("  Memory usage: %d bytes (%d KB)", mem, mem/1024)
		if mem > 0 {
			t.Logf("  Per-directory overhead: %d bytes", mem/uint64(numDirs))
		}
	})

	// Test multiple watchers, one per directory
	t.Run("MultipleWatchers", func(t *testing.T) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)
		baseline := m1.HeapAlloc

		watchers := make([]*fsnotify.Watcher, numDirs)
		for i, dir := range dirs {
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				t.Fatal(err)
			}
			defer watcher.Close()

			if err := watcher.Add(dir); err != nil {
				t.Fatal(err)
			}
			watchers[i] = watcher
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)
		current := m2.HeapAlloc

		var mem uint64
		if current > baseline {
			mem = current - baseline
		} else {
			mem = 0
		}

		t.Logf("Multiple watchers (%d watchers, %d directories):", numDirs, numDirs)
		t.Logf("  Total memory usage: %d bytes (%d KB)", mem, mem/1024)
		if mem > 0 {
			t.Logf("  Per-watcher overhead: %d bytes (%d KB)", mem/uint64(numDirs), (mem/uint64(numDirs))/1024)
		}
	})

	// Test realistic devloop scenario
	t.Run("DevloopScenario", func(t *testing.T) {
		// Simulate a typical devloop config with 5 rules
		rules := []struct {
			name       string
			watchCount int
		}{
			{"backend", 3},  // watches: lib/, cmd/, services/
			{"frontend", 4}, // watches: src/, static/, templates/, dist/
			{"tests", 6},    // watches: test/, spec/, lib/, cmd/, src/, integration/
			{"docs", 2},     // watches: docs/, README.md
			{"protobuf", 1}, // watches: protos/
		}

		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)
		baseline := m1.HeapAlloc

		totalWatchers := 0
		totalDirs := 0
		for _, rule := range rules {
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				t.Fatal(err)
			}
			defer watcher.Close()

			// Add directories for this rule
			for j := 0; j < rule.watchCount && totalDirs < len(dirs); j++ {
				if err := watcher.Add(dirs[totalDirs]); err != nil {
					t.Fatal(err)
				}
				totalDirs++
			}
			totalWatchers++
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)
		current := m2.HeapAlloc

		var mem uint64
		if current > baseline {
			mem = current - baseline
		} else {
			mem = 0
		}

		t.Logf("Realistic devloop scenario (%d rules, %d watchers, %d directories):", len(rules), totalWatchers, totalDirs)
		t.Logf("  Total memory usage: %d bytes (%d KB)", mem, mem/1024)
		if mem > 0 {
			t.Logf("  Per-rule overhead: %d bytes (%d KB)", mem/uint64(totalWatchers), (mem/uint64(totalWatchers))/1024)
		}
	})
}
