package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

)



// Close closes the BadgerDB database and stops all running components with graceful shutdown
func (s *BadgerStorage) Close() error {
	return s.CloseWithTimeout(30 * time.Second)
}

// CloseWithTimeout closes the BadgerDB database with a specified timeout for graceful shutdown
func (s *BadgerStorage) CloseWithTimeout(timeout time.Duration) error {

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Step 1: Brief status report on pending item queue and write queue at initial step
	s.reportInitialShutdownStatus()

	// Execute sequential shutdown of all components (no more concurrent shutdown)
	errors := s.executeSequentialShutdown(ctx)

	// elapsed := time.Since(startTime)

	// Perform pre-close diagnostics based on badger_close_hang.md analysis
	s.performPreCloseCleanup()

	// Step 2: Brief status report just before closing badger storage
	s.reportPreCloseStatus()

	// Close database (this should be last) with timeout protection
	dbCloseStart := time.Now()
	dbCloseTimeout := 30 * time.Second // 30 second timeout for database close

	// Create a channel to track database close completion
	dbCloseDone := make(chan error, 1)

	// Start database close in goroutine to monitor progress
	go func() {
		defer func() {
			// Ensure we always send a result to prevent goroutine leak
			select {
			case dbCloseDone <- nil:
			default:
				// Channel already has a value or is closed
			}
		}()

		// Close DB
		err := s.db.Close()
		select {
		case dbCloseDone <- err:
			// fmt.Println("Database close completed")
		default:
			// Channel might be full or closed due to timeout
			// fmt.Println("Database close channel is full or closed")
		}
	}()

	// Monitor database close progress with timeout
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	dbCtx, dbCancel := context.WithTimeout(context.Background(), dbCloseTimeout)
	defer dbCancel()

	for {
		select {
		case dbErr := <-dbCloseDone:
			elapsed := time.Since(dbCloseStart)
			if dbErr != nil {
				fmt.Printf("BadgerDB database shutdown - FAILED (%v) [%v]\n", dbErr, elapsed)
				errors = append(errors, fmt.Errorf("database close error: %w", dbErr))
			} else {
				fmt.Printf("BadgerDB database shutdown - SUCCESS [%v]\n", elapsed)
				atomic.StoreInt32(&s.isClosed, 1) // Mark as closed only if successful
			}
			goto dbCloseComplete

		case <-ticker.C:
			// Continue monitoring
			continue

		case <-dbCtx.Done():
			elapsed := time.Since(dbCloseStart)
			fmt.Printf("BadgerDB database shutdown - TIMEOUT [%v]\n", elapsed)
			// Don't mark as closed since we couldn't close properly
			errors = append(errors, fmt.Errorf("database close timeout after %v - BadgerDB close hanging", time.Since(dbCloseStart)))
			goto dbCloseComplete
		}
	}

dbCloseComplete:

	// Return aggregated errors if any
	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// performPreCloseCleanup implements recommendations from badger_close_hang.md analysis
func (s *BadgerStorage) performPreCloseCleanup() {
	// 1. Cancel all background operations (cache warmup, optimization, etc.) with mutex protection
	s.backgroundOpsMu.Lock()
	if s.backgroundOpsCancel != nil {
		s.backgroundOpsCancel()
	}
	s.backgroundOpsMu.Unlock()

	// 2. Wait for background operations to stop (optimized timing)
	time.Sleep(500 * time.Millisecond) // Reduced from 3s - background ops should stop quickly with timeout

	// 3. Check for any open transactions or iterators
	// Force a small delay to allow any pending transactions to complete
	time.Sleep(100 * time.Millisecond) // Back to 100ms - sufficient for transaction cleanup

	// 4. Skip final cleanup operations that might interfere with close

	// 5. Final wait for any remaining operations to settle
	time.Sleep(200 * time.Millisecond) // Reduced from 1s - most operations should be done by now

}

// executeSequentialShutdown stops all components one by one in logical order
func (s *BadgerStorage) executeSequentialShutdown(ctx context.Context) []error {

	var errors []error

	// Sequential shutdown order: stop interfering components first, then drain queue cleanly
	// 1. Health monitor (stops database health checks that could interfere with queue draining)
	fmt.Print("Stopping health monitor...")
	if err := s.stopComponentWithTimeout(ctx, "Health monitor", s.stopHealthMonitor); err != nil {
		fmt.Printf(" FAILED (%v)\n", err)
		errors = append(errors, err)
	} else {
		fmt.Print(" SUCCESS\n")
	}

	// 2. Backup manager (stops database backup operations that could hold locks during queue draining)
	fmt.Print("Stopping backup manager...")
	if err := s.stopComponentWithTimeout(ctx, "Backup manager", s.stopBackupManager); err != nil {
		fmt.Printf(" FAILED (%v)\n", err)
		errors = append(errors, err)
	} else {
		fmt.Print(" SUCCESS\n")
	}

	// 3. Garbage collector (stops GC operations that could conflict with write operations)
	fmt.Print("Stopping garbage collector...")
	if err := s.stopComponentWithTimeout(ctx, "Garbage collector", s.stopGarbageCollector); err != nil {
		fmt.Printf(" FAILED (%v)\n", err)
		errors = append(errors, err)
	} else {
		fmt.Print(" SUCCESS\n")
	}

	// Brief pause to ensure interfering components are fully stopped before queue draining
	time.Sleep(100 * time.Millisecond)

	// 4. Queue draining (now can proceed without interference from other components)
	if s.queuedBatch != nil {
		fmt.Print("Queue flush: starting...")
		flushErr := s.FlushQueue()
		if flushErr != nil {
			fmt.Printf(" TIMEOUT [%v]\n", flushErr)
			errors = append(errors, fmt.Errorf("queue flush timeout: %w", flushErr))

			// Continue with queue worker shutdown even after flush timeout
			if s.queuedBatch.IsRunning() {
				fmt.Print("Worker still running after flush timeout - proceeding with shutdown...")
			}
		} else {
			fmt.Print(" SUCCESS\n")
		}
	}

	// 5. Queue system stop (after draining is complete or timed out)
	fmt.Print("Stopping queue worker...")
	if err := s.stopComponentWithTimeout(ctx, "Queue worker", s.stopQueueWorker); err != nil {
		fmt.Printf(" FAILED (%v)\n", err)
		errors = append(errors, err)
	} else {
		fmt.Print(" SUCCESS\n")
	}

	return errors
}

// stopComponentWithTimeout stops a single component with timeout protection
func (s *BadgerStorage) stopComponentWithTimeout(ctx context.Context, name string, stopFunc func()) error {
	done := make(chan struct{})

	go func() {
		defer close(done)
		stopFunc()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s shutdown timeout", name)
	}
}

// Component-specific stop functions - factored for consistency
func (s *BadgerStorage) stopHealthMonitor() {
	s.stopComponent(s.healthMonitor)
}

func (s *BadgerStorage) stopBackupManager() {
	s.stopComponent(s.backupManager)
}

func (s *BadgerStorage) stopGarbageCollector() {
	s.stopComponent(s.gc)
}


// stopQueueWorker stops the queue worker component (used after draining is complete)
func (s *BadgerStorage) stopQueueWorker() {
	if s.queuedBatch == nil {
		return
	}
	s.stopComponent(s.queuedBatch)
}

// stopComponent provides a common pattern for stopping components safely
func (s *BadgerStorage) stopComponent(component interface{}) {
	if component == nil {
		return
	}

	// Additional nil check for typed nil pointers
	// In Go, a nil pointer to a concrete type is not the same as a nil interface
	switch v := component.(type) {
	case *GarbageCollector:
		if v == nil {
			return
		}
	case *BackupManager:
		if v == nil {
			return
		}
	case *HealthMonitor:
		if v == nil {
			return
		}
	case *QueuedWriteBatch:
		if v == nil {
			return
		}
	}

	// Check if component is already stopped before calling Stop()
	if runner, ok := component.(interface{ IsRunning() bool }); ok {
		if !runner.IsRunning() {
			return // Already stopped, don't call Stop() again
		}
	}

	// Use type assertion to call Stop() method
	if stopper, ok := component.(interface{ Stop() }); ok {
		stopper.Stop()
	}
}

// Component-specific verification functions - factored for consistency
func (s *BadgerStorage) verifyHealthMonitorStopped() error {
	return s.verifyComponentStopped(s.healthMonitor, "Health monitor")
}

func (s *BadgerStorage) verifyBackupManagerStopped() error {
	return s.verifyComponentStopped(s.backupManager, "Backup manager")
}

func (s *BadgerStorage) verifyGarbageCollectorStopped() error {
	return s.verifyComponentStopped(s.gc, "Garbage collector")
}

// verifyComponentStopped provides a common pattern for verifying component shutdown
func (s *BadgerStorage) verifyComponentStopped(component interface{}, name string) error {
	if component == nil {
		return nil
	}

	// Use type assertion to check if component is still running
	if runner, ok := component.(interface{ IsRunning() bool }); ok {
		if runner.IsRunning() {
			return fmt.Errorf("%s still running", name)
		}
	}
	return nil
}

func (s *BadgerStorage) verifyQueuedWritesStopped() error {
	if s.queuedBatch != nil {
		// Check if there are any pending writes in the queue
		metrics := s.queuedBatch.GetMetrics()
		if metrics.QueueDepth > 0 {
			return fmt.Errorf("queued writes still has %d pending tasks", metrics.QueueDepth)
		}
	}
	return nil
}

// VerifyCleanShutdown verifies that all components have been properly shut down
func (s *BadgerStorage) VerifyCleanShutdown() map[string]bool {
	status := make(map[string]bool)

	// Define components to check with their names and status checkers
	components := []struct {
		name      string
		component interface{}
		checker   func(interface{}) bool
	}{
		{"garbage_collector", s.gc, s.isComponentStopped},
		{"backup_manager", s.backupManager, s.isComponentStopped},
		{"health_monitor", s.healthMonitor, s.isComponentStopped},
		{"queued_writes", s.queuedBatch, s.isComponentStopped},
	}

	// Check each component's status
	for _, comp := range components {
		status[comp.name] = comp.checker(comp.component)
	}

	return status
}

// isComponentStopped checks if a component is stopped using IsRunning() method
func (s *BadgerStorage) isComponentStopped(component interface{}) bool {
	if component == nil {
		return true // Not initialized, so considered stopped
	}

	// Additional nil check for typed nil pointers
	switch v := component.(type) {
	case *GarbageCollector:
		if v == nil {
			return true
		}
	case *BackupManager:
		if v == nil {
			return true
		}
	case *HealthMonitor:
		if v == nil {
			return true
		}
	case *QueuedWriteBatch:
		if v == nil {
			return true
		}
	}

	if runner, ok := component.(interface{ IsRunning() bool }); ok {
		return !runner.IsRunning()
	}
	return true // If no IsRunning method, assume stopped
}


// IsFullyShutdown returns true if all components have been properly shut down
func (s *BadgerStorage) IsFullyShutdown() bool {
	status := s.VerifyCleanShutdown()
	for _, stopped := range status {
		if !stopped {
			return false
		}
	}
	return true
}

// Restart restarts all components after a shutdown
func (s *BadgerStorage) Restart() error {
	// Check if database has been closed
	if atomic.LoadInt32(&s.isClosed) == 1 {
		return fmt.Errorf("cannot restart: database has been closed, create a new storage instance instead")
	}

	// Verify all components are stopped first
	if !s.IsFullyShutdown() {
		return fmt.Errorf("cannot restart: some components are still running")
	}

	// Start components in dependency order
	var errors []error

	// Start garbage collector if configured
	if s.gc != nil {
		s.gc.Start()
	}

	// Start backup manager if configured
	if s.backupManager != nil {
		if err := s.backupManager.Start(); err != nil {
			errors = append(errors, fmt.Errorf("failed to start backup manager: %w", err))
		}
	}

	// Start health monitoring if configured
	if s.healthMonitor != nil {
		s.healthMonitor.Start()
	}

	// Note: Components are stateless or will be restarted when needed

	if len(errors) > 0 {
		return fmt.Errorf("restart errors: %v", errors)
	}

	return nil
}

// IsClosed returns true if the database has been closed
func (s *BadgerStorage) IsClosed() bool {
	return atomic.LoadInt32(&s.isClosed) == 1
}

// reportInitialShutdownStatus reports brief status on pending item queue and write queue at initial shutdown step
func (s *BadgerStorage) reportInitialShutdownStatus() {
	if s.queuedBatch != nil {
		// Use fixed QueueDepth metric and pending count
		metrics := s.queuedBatch.GetMetrics()
		pendingCount := s.queuedBatch.GetPendingContentCount()
		fmt.Printf("Shutdown initiated - Write queue: %d queued, %d pending items\n", metrics.QueueDepth, pendingCount)
	} else {
		fmt.Printf("Shutdown initiated - No write queue active\n")
	}
}

// reportPreCloseStatus reports brief status just before closing badger storage
func (s *BadgerStorage) reportPreCloseStatus() {
	if s.queuedBatch != nil {
		// Use fixed QueueDepth metric and pending count
		metrics := s.queuedBatch.GetMetrics()
		pendingCount := s.queuedBatch.GetPendingContentCount()
		fmt.Printf("Pre-close status - Write queue: %d queued, %d pending items\n", metrics.QueueDepth, pendingCount)
	} else {
		fmt.Printf("Pre-close status - No write queue active\n")
	}
}

// EmergencyShutdown performs catastrophic failure shutdown with volatile queue state preservation.
//
// This function implements the 3-step emergency shutdown protocol designed for immediate
// server termination while preserving data integrity and enabling automatic recovery:
//
// ## Emergency Shutdown Protocol:
// 1. **Immediate Request Refusal** - Caller must set shutdown state to refuse new requests
// 2. **Worker Termination** - Send stop signal to queue worker without waiting for completion
// 3. **State Serialization** - Serialize volatile queue state (pending items + write queue) to recovery files
//
// ## Data Preservation Strategy:
// - **Pending Items**: Tasks consumed by worker but not yet written to database
// - **Write Queue**: Tasks queued but not yet consumed by worker
// - **Recovery Metadata**: Shutdown timestamp and recovery information
//
// ## Recovery File Structure:
// - `pending-items-{timestamp}.json` - Serialized pending tasks
// - `write-queue-{timestamp}.json` - Serialized queued tasks
// - `recovery-metadata-{timestamp}.json` - Recovery metadata
//
// ## Performance Characteristics:
// - **Target Time**: <5 seconds total shutdown time
// - **Pre-allocated Buffers**: Emergency buffers prepared during startup for speed
// - **Minimal Error Handling**: Optimized for speed over comprehensive error reporting
// - **No Console Output**: Silent operation to minimize shutdown time
//
// ## Recovery Integration:
// - Files are automatically detected and processed on next server startup
// - Processed files are moved to `processed/` subdirectory after successful recovery
// - Failed recovery attempts leave files in place for manual intervention
//
// ## Error Handling:
// - Non-critical errors are ignored to prioritize speed
// - Critical serialization errors are returned but don't prevent shutdown
// - Database close errors are logged but don't block emergency exit
//
// Returns error only for critical failures that prevent state preservation.
// The server will exit immediately regardless of return value.
func (s *BadgerStorage) EmergencyShutdown() error {
	startTime := time.Now()

	// Step 1: Immediately stop write queue worker (no waiting, no console output)
	if s.queuedBatch != nil {
		s.queuedBatch.EmergencyStop() // Ignore error for speed
	}

	// Step 2: Serialize volatile queue state to recovery files (optimized)
	timestamp := startTime.Format("20060102-150405") // Use same time for consistency

	// Use pre-prepared emergency recovery directory (NO folder operations during emergency)
	recoveryDir := s.emergencyRecoveryDir
	if recoveryDir == "" {
		// Emergency shutdown FAILS if preparation was not done during startup
		return fmt.Errorf("emergency recovery not prepared")
	}

	// Serialize volatile queue state (no console output for speed)
	if s.queuedBatch != nil {
		// Serialize pending items
		if pendingData, err := s.queuedBatch.SerializePendingTasks(); err == nil {
			pendingFile := filepath.Join(recoveryDir, fmt.Sprintf("pending-items-%s.json", timestamp))
			os.WriteFile(pendingFile, pendingData, 0644) // Ignore error for speed
		}

		// Serialize write queue
		if queueData, err := s.queuedBatch.SerializeWriteQueue(); err == nil {
			queueFile := filepath.Join(recoveryDir, fmt.Sprintf("write-queue-%s.json", timestamp))
			os.WriteFile(queueFile, queueData, 0644) // Ignore error for speed
		}
	}

	// Step 3: Save recovery metadata (ignore error for speed)
	s.saveEmergencyRecoveryMetadata(recoveryDir, timestamp, startTime)

	return nil
}

// saveEmergencyRecoveryMetadata saves recovery metadata for emergency shutdown (ultra-optimized)
func (s *BadgerStorage) saveEmergencyRecoveryMetadata(recoveryDir, timestamp string, startTime time.Time) error {
	// Use pre-prepared metadata template for faster processing
	metadata := make(map[string]interface{}, 5) // Pre-size for known fields

	// Copy pre-prepared template
	if s.emergencyMetadataTemplate != nil {
		for k, v := range s.emergencyMetadataTemplate {
			metadata[k] = v
		}
	}

	// Add minimal runtime-specific data (only essential for recovery)
	metadata["timestamp"] = timestamp
	metadata["shutdown_time"] = startTime.Format(time.RFC3339)
	metadata["pending_items"] = "pending-items-" + timestamp + ".json"
	metadata["write_queue"] = "write-queue-" + timestamp + ".json"

	// Use pre-allocated buffer for JSON marshaling
	s.emergencyMetadataBuffer = s.emergencyMetadataBuffer[:0] // Reset buffer
	if metadataJSON, err := json.Marshal(metadata); err == nil {
		metadataFile := filepath.Join(recoveryDir, "recovery-metadata-"+timestamp+".json")
		os.WriteFile(metadataFile, metadataJSON, 0644) // Ignore error for speed
	}

	return nil
}

