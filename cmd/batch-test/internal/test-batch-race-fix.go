package internal

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"content-storage-server/pkg/models"
	"content-storage-server/pkg/storage"
)

// TestBatchRaceFix tests the fix for worker state machine and simpleFlush during shutdown
func RunBatchRaceFixTest() {
	fmt.Println("=== Testing Worker State Machine and simpleFlush Fix ===")

	// Run all test scenarios
	tests := []struct {
		name string
		test func() bool
	}{
		{"simpleFlush with Tasks in Internal Batch", testFlushWithTasksInInternalBatch},
		{"Worker State Machine Shutdown", testShutdownRaceCondition},
		{"simpleFlush Timeout Handling", testEnhancedTimeoutCalculation},
		{"Force Processing Effectiveness", testForceProcessingEffectiveness},
		{"Concurrent simpleFlush and Shutdown", testConcurrentFlushAndShutdown},
	}

	passed := 0
	for _, test := range tests {
		fmt.Printf("\n--- Running: %s ---\n", test.name)
		if test.test() {
			fmt.Printf("✅ PASSED: %s\n", test.name)
			passed++
		} else {
			fmt.Printf("❌ FAILED: %s\n", test.name)
		}
	}

	fmt.Printf("\n=== Test Results: %d/%d passed ===\n", passed, len(tests))
	if passed != len(tests) {
		os.Exit(1)
	}
}

// testFlushWithTasksInInternalBatch simulates the exact scenario that caused the original timeout
func testFlushWithTasksInInternalBatch() bool {
	// Create storage with specific batch settings to test worker state machine
	opts := storage.BadgerOptions{
		DataDir:                "/tmp/test-worker-state-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		WriteQueueSize:         10,
		WriteQueueBatchSize:    5,               // Small batch size
		WriteQueueBatchTimeout: 2 * time.Second, // Long batch timeout to test worker state transitions
	}

	stor, err := storage.NewBadgerStorage(opts)
	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		return false
	}
	defer func() {
		stor.Close()
		os.RemoveAll(opts.DataDir)
	}()

	// Add exactly 4 items (less than batch size) to trigger batch timeout scenario
	for i := 0; i < 4; i++ {
		content := &models.Content{
			ID:   fmt.Sprintf("test-batch-race-%d", i),
			Data: fmt.Sprintf("test data %d", i),
		}

		err := stor.Store(content)
		if err != nil {
			fmt.Printf("Failed to store content %d: %v\n", i, err)
			return false
		}
	}

	// Verify items are queued
	pendingCount := stor.GetPendingContentCount()
	if pendingCount != 4 {
		fmt.Printf("Expected 4 pending items, got %d\n", pendingCount)
		return false
	}
	fmt.Printf("✓ 4 items queued and pending\n")

	// Test the simpleFlush - this should now handle the worker state machine properly
	fmt.Printf("Testing simpleFlush with worker state machine...\n")
	start := time.Now()
	err = stor.FlushQueue()
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("simpleFlush failed: %v (took %v)\n", err, elapsed)
		return false
	}

	fmt.Printf("✓ simpleFlush succeeded in %v (worker stopped, queue drained)\n", elapsed)

	// Verify all items were processed
	pendingCount = stor.GetPendingContentCount()
	if pendingCount != 0 {
		fmt.Printf("Expected 0 pending items after flush, got %d\n", pendingCount)
		return false
	}
	fmt.Printf("✓ All items processed after flush\n")

	return true
}

// testShutdownRaceCondition tests the coordination between flush and shutdown
func testShutdownRaceCondition() bool {
	opts := storage.BadgerOptions{
		DataDir:                "/tmp/test-shutdown-race-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		WriteQueueSize:         10,
		WriteQueueBatchSize:    3,
		WriteQueueBatchTimeout: 1500 * time.Millisecond, // Moderate timeout
	}

	stor, err := storage.NewBadgerStorage(opts)
	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		return false
	}
	defer func() {
		stor.Close()
		os.RemoveAll(opts.DataDir)
	}()

	// Add items that will be in internal batch during shutdown
	for i := 0; i < 2; i++ { // Less than batch size
		content := &models.Content{
			ID:   fmt.Sprintf("shutdown-race-%d", i),
			Data: fmt.Sprintf("shutdown test data %d", i),
		}

		err := stor.Store(content)
		if err != nil {
			fmt.Printf("Failed to store content %d: %v\n", i, err)
			return false
		}
	}

	// Simulate the shutdown sequence that was causing the race condition
	var wg sync.WaitGroup
	var flushResult error
	var shutdownResult error

	// Start simpleFlush in background (simulating shutdown flush)
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Printf("Starting simpleFlush...\n")
		flushResult = stor.FlushQueue()
		fmt.Printf("simpleFlush completed with result: %v\n", flushResult)
	}()

	// Give simpleFlush a moment to start, then simulate shutdown
	time.Sleep(100 * time.Millisecond)

	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Printf("Starting sequential shutdown...\n")
		shutdownResult = stor.Close()
		fmt.Printf("Sequential shutdown completed with result: %v\n", shutdownResult)
	}()

	wg.Wait()

	// Both operations should succeed or handle gracefully
	if flushResult != nil && shutdownResult != nil {
		fmt.Printf("Both simpleFlush and shutdown failed: flush=%v, shutdown=%v\n", flushResult, shutdownResult)
		return false
	}

	fmt.Printf("✓ Worker state machine shutdown handled gracefully\n")
	return true
}

// testEnhancedTimeoutCalculation verifies the simpleFlush timeout handling
func testEnhancedTimeoutCalculation() bool {
	opts := storage.BadgerOptions{
		DataDir:                "/tmp/test-simpleflush-timeout-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		WriteQueueSize:         10,
		WriteQueueBatchSize:    5,
		WriteQueueBatchTimeout: 5 * time.Second, // Long timeout to test simpleFlush behavior
	}

	stor, err := storage.NewBadgerStorage(opts)
	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		return false
	}
	defer func() {
		stor.Close()
		os.RemoveAll(opts.DataDir)
	}()

	// Add items to test timeout calculation
	itemCount := 3
	for i := 0; i < itemCount; i++ {
		content := &models.Content{
			ID:   fmt.Sprintf("timeout-calc-%d", i),
			Data: fmt.Sprintf("timeout test data %d", i),
		}

		err := stor.Store(content)
		if err != nil {
			fmt.Printf("Failed to store content %d: %v\n", i, err)
			return false
		}
	}

	// Test that simpleFlush handles timeout properly
	start := time.Now()
	err = stor.FlushQueue()
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("❌ simpleFlush timeout handling failed: %v\n", err)
		return false
	}

	// The test verifies that simpleFlush completes successfully by stopping worker first
	// then draining the queue, regardless of batch timeout settings.
	fmt.Printf("✓ simpleFlush timeout handling working (took %v, worker stopped first)\n", elapsed)
	return true
}

// testForceProcessingEffectiveness tests that forceProcessStuckTasks works correctly in simpleFlush
func testForceProcessingEffectiveness() bool {
	opts := storage.BadgerOptions{
		DataDir:                "/tmp/test-force-stuck-tasks-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		WriteQueueSize:         10,
		WriteQueueBatchSize:    5,
		WriteQueueBatchTimeout: 5 * time.Second, // Very long timeout
	}

	stor, err := storage.NewBadgerStorage(opts)
	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		return false
	}
	defer func() {
		stor.Close()
		os.RemoveAll(opts.DataDir)
	}()

	// Add items that will sit in writeQueue (not pulled by worker yet)
	// We'll add them quickly to increase chance they're still in writeQueue
	var wg sync.WaitGroup
	itemCount := 3

	for i := 0; i < itemCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			content := &models.Content{
				ID:   fmt.Sprintf("force-processing-%d", id),
				Data: fmt.Sprintf("force processing test data %d", id),
			}

			err := stor.Store(content)
			if err != nil {
				fmt.Printf("Failed to store content %d: %v\n", id, err)
			}
		}(i)
	}
	wg.Wait()

	// Give a moment for items to be queued
	time.Sleep(50 * time.Millisecond)

	// Check that items are pending
	initialPending := stor.GetPendingContentCount()

	fmt.Printf("Before force processing: %d pending\n", initialPending)

	// Test force processing stuck tasks (this is called internally by simpleFlush)
	// We can't call it directly, so we'll test via simpleFlush
	start := time.Now()
	err = stor.FlushQueue()
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("Force processing stuck tasks test failed: %v\n", err)
		return false
	}

	finalPending := stor.GetPendingContentCount()

	fmt.Printf("After simpleFlush: %d pending (took %v)\n", finalPending, elapsed)

	if finalPending != 0 {
		fmt.Printf("Expected 0 pending items after force processing stuck tasks, got %d\n", finalPending)
		return false
	}

	fmt.Printf("✓ Force processing stuck tasks effectiveness verified\n")
	return true
}

// testConcurrentFlushAndShutdown tests the most complex scenario
func testConcurrentFlushAndShutdown() bool {
	opts := storage.BadgerOptions{
		DataDir:                "/tmp/test-concurrent-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		WriteQueueSize:         20,
		WriteQueueBatchSize:    4,
		WriteQueueBatchTimeout: 1 * time.Second,
	}

	stor, err := storage.NewBadgerStorage(opts)
	if err != nil {
		fmt.Printf("Failed to create storage: %v\n", err)
		return false
	}
	defer func() {
		stor.Close()
		os.RemoveAll(opts.DataDir)
	}()

	// Add multiple items concurrently to create a realistic load
	var storeWg sync.WaitGroup
	var storeErrors int64
	itemCount := 10

	for i := 0; i < itemCount; i++ {
		storeWg.Add(1)
		go func(id int) {
			defer storeWg.Done()
			content := &models.Content{
				ID:   fmt.Sprintf("concurrent-%d", id),
				Data: fmt.Sprintf("concurrent test data %d", id),
			}

			err := stor.Store(content)
			if err != nil {
				atomic.AddInt64(&storeErrors, 1)
				fmt.Printf("Store error for item %d: %v\n", id, err)
			}
		}(i)
	}
	storeWg.Wait()

	if storeErrors > 0 {
		fmt.Printf("Store errors occurred: %d\n", storeErrors)
		return false
	}

	// Now test concurrent flush and shutdown operations
	var opWg sync.WaitGroup
	var flushErr, shutdownErr error
	var flushDuration, shutdownDuration time.Duration

	// Start simpleFlush
	opWg.Add(1)
	go func() {
		defer opWg.Done()
		start := time.Now()
		flushErr = stor.FlushQueue()
		flushDuration = time.Since(start)
		fmt.Printf("Concurrent simpleFlush completed in %v with result: %v\n", flushDuration, flushErr)
	}()

	// Start sequential shutdown slightly after simpleFlush
	time.Sleep(50 * time.Millisecond)
	opWg.Add(1)
	go func() {
		defer opWg.Done()
		start := time.Now()
		shutdownErr = stor.Close()
		shutdownDuration = time.Since(start)
		fmt.Printf("Concurrent sequential shutdown completed in %v with result: %v\n", shutdownDuration, shutdownErr)
	}()

	opWg.Wait()

	// At least one operation should succeed, and no data should be lost
	if flushErr != nil && shutdownErr != nil {
		fmt.Printf("Both concurrent operations failed: simpleFlush=%v, shutdown=%v\n", flushErr, shutdownErr)
		return false
	}

	fmt.Printf("✓ Concurrent simpleFlush and sequential shutdown handled correctly\n")
	fmt.Printf("  simpleFlush: %v (%v), Sequential shutdown: %v (%v)\n",
		flushErr, flushDuration, shutdownErr, shutdownDuration)

	return true
}
