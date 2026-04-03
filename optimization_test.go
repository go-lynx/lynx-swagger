package swagger

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-openapi/spec"
	"github.com/stretchr/testify/assert"
)

func TestSafeStringParsing(t *testing.T) {
	parser := NewAnnotationParser(&spec.Swagger{}, []string{"./"})

	// Test safe split with normal input
	parts := parser.safeSplit("a,b,c", ",")
	assert.Equal(t, []string{"a", "b", "c"}, parts)

	// Test safe split with empty input
	parts = parser.safeSplit("", ",")
	assert.Nil(t, parts)

	// Test safe split with very long input
	longInput := string(make([]byte, 2000))
	parts = parser.safeSplit(longInput, ",")
	assert.Nil(t, parts)

	// Test safe fields
	fields := parser.safeFields("a b c")
	assert.Equal(t, []string{"a", "b", "c"}, fields)
}

func TestMemoryManagement(t *testing.T) {
	parser := NewAnnotationParser(&spec.Swagger{}, []string{"./"})

	// Test initial memory stats
	stats := parser.GetMemoryStats()
	assert.Equal(t, 0, stats["models_count"])
	assert.Equal(t, 0, stats["routes_count"])

	// Test memory optimization
	parser.OptimizeMemory()

	// Test cache clearing
	parser.ClearCache()
	stats = parser.GetMemoryStats()
	assert.Equal(t, 0, stats["models_count"])
	assert.Equal(t, 0, stats["routes_count"])
}

func TestFileWatcherConfig(t *testing.T) {
	config := FileWatcherConfig{
		Enabled:       true,
		Interval:      1 * time.Second,
		DebounceDelay: 500 * time.Millisecond,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		BatchSize:     10,
		HealthCheck:   true,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 1*time.Second, config.Interval)
	assert.Equal(t, 500*time.Millisecond, config.DebounceDelay)
	assert.Equal(t, 3, config.MaxRetries)
}

func TestStringBuilderPool(t *testing.T) {
	pool := NewStringBuilderPool(5)

	// Test getting and putting string builders
	sb1 := pool.Get()
	sb1.WriteString("test")
	assert.Equal(t, "test", sb1.String())

	pool.Put(sb1)

	// Test getting again
	sb2 := pool.Get()
	assert.Equal(t, "", sb2.String()) // Should be reset
}

func TestParseStats(t *testing.T) {
	parser := NewAnnotationParser(&spec.Swagger{}, []string{"./"})

	// Test initial stats
	stats := parser.GetStats()
	assert.Equal(t, 0, stats.TotalFiles)
	assert.Equal(t, 0, stats.SuccessFiles)
	assert.Equal(t, 0, stats.FailedFiles)

	// Test error summary
	summary := parser.GetErrorSummary()
	assert.Empty(t, summary)
}

func TestPlugSwaggerCleanupTasksStopsWatcherOnce(t *testing.T) {
	plugin := NewSwaggerPlugin()
	plugin.watcher = &FileWatcher{
		stop: make(chan struct{}),
		config: FileWatcherConfig{
			Interval:      10 * time.Millisecond,
			DebounceDelay: 10 * time.Millisecond,
			MaxRetries:    1,
			RetryDelay:    10 * time.Millisecond,
		},
		callback: func() error { return nil },
		healthy:  true,
	}

	done := make(chan struct{})
	go func() {
		plugin.watcher.watch()
		close(done)
	}()

	if err := plugin.CleanupTasks(); err != nil {
		t.Fatalf("first cleanup failed: %v", err)
	}
	if err := plugin.CleanupTasks(); err != nil {
		t.Fatalf("second cleanup failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watcher did not stop after cleanup")
	}
}

func TestFileWatcherProcessChangesStopsDuringRetryBackoff(t *testing.T) {
	var attempts atomic.Int32
	firstAttempt := make(chan struct{})

	watcher := &FileWatcher{
		stop: make(chan struct{}),
		config: FileWatcherConfig{
			MaxRetries: 3,
			RetryDelay: 250 * time.Millisecond,
		},
		callback: func() error {
			if attempts.Add(1) == 1 {
				close(firstAttempt)
			}
			return errors.New("rebuild failed")
		},
		healthy: true,
	}
	watcher.mu.Lock()
	watcher.changeCount = 1
	watcher.mu.Unlock()

	done := make(chan struct{})
	go func() {
		watcher.processChanges()
		close(done)
	}()

	select {
	case <-firstAttempt:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("callback was not invoked")
	}

	stopTime := time.Now()
	watcher.stopWatching()

	select {
	case <-done:
		if time.Since(stopTime) >= 150*time.Millisecond {
			t.Fatal("processChanges did not stop promptly after stop signal")
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("processChanges remained blocked during retry backoff")
	}

	assert.Equal(t, int32(1), attempts.Load())
}
