package storage

import (
	"time"
)

// Queue Management API - Wrapper functions exposing the sequential write queue system
// to the rest of the codebase with comprehensive monitoring and control capabilities.

// GetQueueMetrics retrieves comprehensive metrics for the sequential write queue system.
//
// This function provides detailed operational metrics for monitoring queue performance,
// identifying bottlenecks, and ensuring optimal system operation.
//
// ## Metrics Categories:
// - **Queue Depth**: Current number of items in write queue
// - **Processing Rate**: Tasks processed per second
// - **Batch Statistics**: Batch size distribution and timing
// - **Error Rates**: Failed operations and retry statistics
// - **Performance Metrics**: Processing latency and throughput
//
// ## Monitoring Integration:
// - Used by management dashboard for real-time queue monitoring
// - Integrated with health checks for queue health assessment
// - Provides data for alerting and capacity planning
//
// ## Performance Characteristics:
// - **Fast Execution**: Optimized for frequent metric collection
// - **Thread-safe**: Safe for concurrent access from multiple goroutines
// - **Non-blocking**: Does not interfere with queue operations
//
// Returns QueueMetrics pointer with current statistics, or nil if queue not initialized.
func (s *BadgerStorage) GetQueueMetrics() *QueueMetrics {
	if s.queuedBatch != nil {
		metrics := s.queuedBatch.GetMetrics()
		return &metrics
	}
	return nil
}

// IsContentInQueue checks if content with the given ID is currently in the write queue
func (s *BadgerStorage) IsContentInQueue(contentID string) bool {
	if s.queuedBatch != nil {
		return s.queuedBatch.IsContentInQueue(contentID)
	}
	return false
}

// GetPendingContent retrieves the pending WriteTask for the given content ID from the queue
// Returns the task and true if found, nil and false if not found
func (s *BadgerStorage) GetPendingContent(contentID string) (*WriteTask, bool) {
	if s.queuedBatch != nil {
		return s.queuedBatch.GetPendingContent(contentID)
	}
	return nil, false
}

// GetPendingContentIDs returns a slice of all content IDs currently pending in the write queue
func (s *BadgerStorage) GetPendingContentIDs() []string {
	if s.queuedBatch != nil {
		return s.queuedBatch.GetPendingContentIDs()
	}
	return []string{}
}

// GetPendingContentCount returns the number of items currently pending in the write queue
func (s *BadgerStorage) GetPendingContentCount() int {
	if s.queuedBatch != nil {
		return s.queuedBatch.GetPendingContentCount()
	}
	return 0
}

// WaitForContentWrite waits for a specific content ID to complete writing
// Returns nil if write completes successfully, error if timeout or write fails
func (s *BadgerStorage) WaitForContentWrite(contentID string, timeout time.Duration) error {
	if s.queuedBatch == nil {
		return nil // No queue system, nothing to wait for
	}
	return s.queuedBatch.WaitForContentWrite(contentID, timeout)
}

// FlushQueue forces immediate processing of all pending write operations.
//
// This function implements a coordinated queue flush that ensures all pending
// write operations are completed before returning, providing strong consistency
// guarantees for critical operations.
//
// ## Flush Process:
// 1. **Worker Coordination** - Coordinates with queue worker for graceful flush
// 2. **Queue Drainage** - Processes all items currently in write queue
// 3. **Pending Completion** - Ensures all pending items are written to database
// 4. **Consistency Wait** - Waits for database operations to complete
//
// ## Consistency Guarantees:
// - **All Queued Items**: Processes all items in write queue at flush time
// - **Pending Items**: Completes all items consumed but not yet processed
// - **Database Sync**: Ensures database operations are committed
// - **Atomic Completion**: Either all items are processed or error is returned
//
// ## Use Cases:
// - **Pre-shutdown**: Ensures clean shutdown with no data loss
// - **Critical Operations**: Guarantees consistency before important operations
// - **Testing**: Ensures deterministic state for test validation
// - **Manual Sync**: Provides manual control over queue processing
//
// ## Performance Impact:
// - **Blocking Operation**: Blocks until all pending operations complete
// - **Resource Intensive**: May cause temporary performance impact
// - **Timeout Protection**: Uses timeouts to prevent indefinite blocking
//
// ## Error Handling:
// - **Partial Success**: May complete some operations even if others fail
// - **Timeout Handling**: Returns error if flush cannot complete within timeout
// - **Resource Cleanup**: Ensures proper cleanup even on failure
//
// Returns error if flush cannot be completed successfully.
func (s *BadgerStorage) FlushQueue() error {
	if s.queuedBatch == nil {
		return nil // No queue to flush
	}
	return s.queuedBatch.simpleFlush()
}
