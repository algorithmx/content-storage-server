package internal

import (
	"fmt"
	"os"
	"time"

	"content-storage-server/pkg/models"
	"content-storage-server/pkg/storage"
)

func RunSimpleBatchFixTest() {
	fmt.Println("=== Testing Batch Race Condition Fix ===")

	// Create storage with the exact configuration that was causing the issue
	opts := storage.BadgerOptions{
		DataDir:                "/tmp/test-simple-batch-fix",
		WriteQueueSize:         10,
		WriteQueueBatchSize:    5,               // Batch size of 5
		WriteQueueBatchTimeout: 2 * time.Second, // 2 second timeout
	}

	stor, err := storage.NewBadgerStorage(opts)
	if err != nil {
		fmt.Printf("❌ Failed to create storage: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		stor.Close()
		os.RemoveAll(opts.DataDir)
	}()

	// Add exactly 4 items (the scenario from your logs)
	fmt.Println("Adding 4 items (less than batch size of 5)...")
	for i := 0; i < 4; i++ {
		content := &models.Content{
			ID:   fmt.Sprintf("test-item-%d", i),
			Data: fmt.Sprintf("test data %d", i),
		}

		err := stor.Store(content)
		if err != nil {
			fmt.Printf("❌ Failed to store content %d: %v\n", i, err)
			os.Exit(1)
		}
	}

	// Check pending count
	pendingCount := stor.GetPendingContentCount()
	fmt.Printf("✓ %d items queued and pending\n", pendingCount)

	if pendingCount != 4 {
		fmt.Printf("❌ Expected 4 pending items, got %d\n", pendingCount)
		os.Exit(1)
	}

	// Test the simpleFlush - this should now work with the worker state machine fix
	fmt.Println("Testing simpleFlush with worker state machine (this was timing out before the fix)...")
	start := time.Now()
	err = stor.FlushQueue()
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("❌ simpleFlush failed: %v (took %v)\n", err, elapsed)
		fmt.Println("   This means the worker state machine fix didn't work!")
		os.Exit(1)
	}

	fmt.Printf("✅ simpleFlush succeeded in %v (worker stopped, queue drained)\n", elapsed)

	// Verify all items were processed
	finalPending := stor.GetPendingContentCount()
	if finalPending != 0 {
		fmt.Printf("❌ Expected 0 pending items after flush, got %d\n", finalPending)
		os.Exit(1)
	}

	fmt.Println("✅ All items processed successfully!")
	fmt.Println()
	fmt.Println("=== Worker State Machine and simpleFlush Fix Test PASSED ===")
	fmt.Printf("Original issue: 4 items stuck in internal batch with 2s timeout\n")
	fmt.Printf("Fix result: All items processed successfully in %v\n", elapsed)
	fmt.Printf("Worker state machine and simpleFlush working correctly!\n")
}
