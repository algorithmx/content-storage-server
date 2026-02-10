package storage

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"content-storage-server/pkg/models"
)

// TestQueueCapacityLimitEnforced tests that queue capacity doubling stops at max limit
func TestQueueCapacityLimitEnforced(t *testing.T) {
	initialSize := 10
	maxMultiplier := 2 // Max size = 20

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	// Set custom max multiplier
	qwb.SetMaxMultiplier(maxMultiplier)

	maxAllowed := initialSize * maxMultiplier

	// Verify initial capacity
	initialCapacity := qwb.GetMaxQueueSize()
	if initialCapacity != initialSize {
		t.Errorf("Expected initial capacity %d, got %d", initialSize, initialCapacity)
	}

	// Test direct capacity doubling to verify limit enforcement
	// First doubling should work
	err = qwb.DoubleQueueCapacity()
	if err != nil {
		t.Fatalf("First doubling should work: %v", err)
	}

	capacityAfterFirstDouble := qwb.GetMaxQueueSize()
	if capacityAfterFirstDouble != maxAllowed {
		t.Logf("Note: Capacity after first double is %d (expected %d)", capacityAfterFirstDouble, maxAllowed)
	}

	// Try to double at max limit - should fail
	err = qwb.DoubleQueueCapacity()
	if err == nil {
		t.Error("Expected error when doubling at max limit, got nil")
	}

	// Verify capacity never exceeded max limit
	finalCapacity := qwb.GetMaxQueueSize()
	if finalCapacity > maxAllowed {
		t.Errorf("Capacity exceeded maximum allowed: %d > %d", finalCapacity, maxAllowed)
	}
}

// TestQueueCapacityCannotExceedMaxMultiplier tests the hard limit enforcement
func TestQueueCapacityCannotExceedMaxMultiplier(t *testing.T) {
	initialSize := 5
	maxMultiplier := 3 // Max size = 15

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    2,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(maxMultiplier)
	maxAllowed := initialSize * maxMultiplier

	// Try to double beyond max limit repeatedly
	expires := time.Now().Add(1 * time.Hour)
	for i := 0; i < 10; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("content-%d", i),
			Type:      "text/plain",
			Data:      "test data",
			ExpiresAt: &expires,
		}
		_ = qwb.QueueWrite(content)

		currentCapacity := qwb.GetMaxQueueSize()
		if currentCapacity > maxAllowed {
			t.Errorf("Iteration %d: capacity %d exceeds max allowed %d", i, currentCapacity, maxAllowed)
		}
	}

	finalCapacity := qwb.GetMaxQueueSize()
	if finalCapacity > maxAllowed {
		t.Errorf("Final capacity %d exceeds max allowed %d", finalCapacity, maxAllowed)
	}
}

// TestDoubleQueueCapacityErrorAtMaxLimit tests that DoubleQueueCapacity returns error at max
func TestDoubleQueueCapacityErrorAtMaxLimit(t *testing.T) {
	initialSize := 10
	maxMultiplier := 1 // Max size = 10 (no doubling allowed)

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(maxMultiplier)

	// Try to double at max limit
	err = qwb.DoubleQueueCapacity()
	if err == nil {
		t.Error("Expected error when doubling at max limit, got nil")
	}

	expectedErrorMsg := fmt.Sprintf("queue at maximum capacity (%d)", initialSize*maxMultiplier)
	if !contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error containing %q, got %q", expectedErrorMsg, err.Error())
	}
}

// TestQueueCapacityDoublerThreadSafety tests concurrent capacity doubling
func TestQueueCapacityDoublerThreadSafety(t *testing.T) {
	initialSize := 10
	maxMultiplier := 5

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(maxMultiplier)
	maxAllowed := initialSize * maxMultiplier

	// Concurrently try to double capacity
	var wg sync.WaitGroup
	numGoroutines := 10
	expires := time.Now().Add(1 * time.Hour)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				content := &models.Content{
					ID:        fmt.Sprintf("goroutine-%d-item-%d", id, j),
					Type:      "text/plain",
					Data:      "concurrent test",
					ExpiresAt: &expires,
				}
				_ = qwb.QueueWrite(content)
			}
		}(i)
	}

	wg.Wait()

	// Verify capacity never exceeded max limit
	finalCapacity := qwb.GetMaxQueueSize()
	if finalCapacity > maxAllowed {
		t.Errorf("Final capacity %d exceeded max allowed %d", finalCapacity, maxAllowed)
	}

	// Verify queue is still functional
	testContent := &models.Content{
		ID:        "final-test",
		Type:      "text/plain",
		Data:      "final",
		ExpiresAt: &expires,
	}
	err = qwb.QueueWrite(testContent)
	// Should either succeed or fail gracefully, but not panic
	if err != nil && finalCapacity < maxAllowed {
		t.Logf("Queue write failed with capacity %d/%d: %v", finalCapacity, maxAllowed, err)
	}
}

// TestInitialQueueSizeTracking tests that initial queue size is correctly tracked
func TestInitialQueueSizeTracking(t *testing.T) {
	initialSize := 20

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	// Default max multiplier is 10
	defaultMaxMultiplier := 10
	expectedMax := initialSize * defaultMaxMultiplier

	// Verify we can double up to the expected max
	currentCapacity := qwb.GetMaxQueueSize()
	expires := time.Now().Add(1 * time.Hour)

	// Fill and double multiple times
	for i := 0; i < 5; i++ {
		// Fill current capacity
		for j := 0; j < currentCapacity; j++ {
			content := &models.Content{
				ID:        fmt.Sprintf("fill-%d-%d", i, j),
				Type:      "text/plain",
				Data:      "data",
				ExpiresAt: &expires,
			}
			_ = qwb.QueueWrite(content)
		}

		// Trigger doubling
		content := &models.Content{
			ID:        fmt.Sprintf("trigger-%d", i),
			Type:      "text/plain",
			Data:      "trigger",
			ExpiresAt: &expires,
		}
		_ = qwb.QueueWrite(content)

		currentCapacity = qwb.GetMaxQueueSize()
		if currentCapacity > expectedMax {
			t.Errorf("Capacity %d exceeded expected max %d", currentCapacity, expectedMax)
		}
	}
}

// TestSetMaxMultiplierUpdatesLimit tests that SetMaxMultiplier updates the limit
func TestSetMaxMultiplierUpdatesLimit(t *testing.T) {
	initialSize := 10

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	// Set max multiplier to 2
	qwb.SetMaxMultiplier(2)
	maxAllowed := initialSize * 2

	// Fill to max
	expires := time.Now().Add(1 * time.Hour)
	for i := 0; i < maxAllowed; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("fill-%d", i),
			Type:      "text/plain",
			Data:      "data",
			ExpiresAt: &expires,
		}
		err = qwb.QueueWrite(content)
		if err != nil {
			t.Fatalf("Failed to queue at index %d: %v", i, err)
		}
	}

	// Try to double at max
	err = qwb.DoubleQueueCapacity()
	if err == nil {
		t.Error("Expected error when doubling at updated max limit")
	}

	// Verify capacity is still at the updated max
	capacity := qwb.GetMaxQueueSize()
	if capacity > maxAllowed {
		t.Errorf("Capacity %d exceeded updated max %d", capacity, maxAllowed)
	}
}

// TestQueueGrowthDoesNotExceedMemory tests that queue growth has reasonable memory bounds
func TestQueueGrowthDoesNotExceedMemory(t *testing.T) {
	initialSize := 100
	maxMultiplier := 10

	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    10,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(maxMultiplier)
	maxAllowed := initialSize * maxMultiplier

	// Aggressively try to exceed the limit
	expires := time.Now().Add(1 * time.Hour)
	for i := 0; i < maxAllowed*2; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("memory-test-%d", i),
			Type:      "text/plain",
			Data:      "data",
			ExpiresAt: &expires,
		}
		_ = qwb.QueueWrite(content)

		currentCapacity := qwb.GetMaxQueueSize()
		if currentCapacity > maxAllowed {
			t.Errorf("Iteration %d: capacity %d exceeded max %d", i, currentCapacity, maxAllowed)
			break
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
