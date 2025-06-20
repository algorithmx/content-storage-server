package storage

import (
	"content-storage-server/pkg/models"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// WriteTask represents a queued write operation
type WriteTask struct {
	Content       *models.Content
	MarshaledData []byte     // Pre-marshaled JSON data to avoid repeated marshaling
	ResultChan    chan error // Optional channel for synchronous operations
}

// QueuedWriteBatch manages asynchronous write operations with intelligent batching
// Uses a single writer to ensure sequential ordering of operations
type QueuedWriteBatch struct {
	writeQueue   chan *WriteTask         // Buffered channel for write tasks
	maxQueueSize int                     // Maximum queue size for backpressure
	batchTimeout time.Duration           // Maximum time to wait before flushing batch
	maxBatchSize int                     // Maximum number of items per batch
	stopChan     chan struct{}           // Channel to stop worker
	stopOnce     sync.Once               // Ensure stop is called only once
	workerWg     sync.WaitGroup          // Wait group for workers
	metrics      QueueMetrics            // Performance metrics (embedded directly)
	errorHandler func(*WriteTask, error) // Error handling callback
	db           *badger.DB              // BadgerDB instance
	pendingItems *CountedSyncMap         // Tracks pending items by content ID with efficient counting

	// Unified notification system (true consolidation: 4 fields → 1 field)
	notifications *UnifiedNotificationSystem // Single system for all notifications

	// Synchronization for queue capacity operations
	capacityMutex sync.Mutex // Protects queue capacity doubling operations

}

// QueuedWriteBatchOptions contains configuration for the queued write system
type QueuedWriteBatchOptions struct {
	QueueSize    int                     // Size of the write queue buffer
	BatchSize    int                     // Maximum items per batch
	BatchTimeout time.Duration           // Maximum time to wait for batch
	ErrorHandler func(*WriteTask, error) // Custom error handler
}

// QueueMetrics tracks queue performance using atomic operations for thread-safety
// All fields are accessed atomically to avoid mixed synchronization patterns
type QueueMetrics struct {
	QueueDepth      int64 // Current queue depth (accessed atomically)
	TotalQueued     int64 // Total tasks queued (accessed atomically)
	TotalProcessed  int64 // Total tasks processed (accessed atomically)
	TotalErrors     int64 // Total errors encountered (accessed atomically)
	processingCount int64 // Count of processing operations for proper averaging (accessed atomically)
}

// workerState defines the possible states of the worker goroutine.
type workerState int

const (
	workerStateRunning         workerState = iota // Worker is actively processing tasks
	workerStatePausingForFlush                    // Worker is paused, waiting for an explicit flush to complete
	workerStateStopping                           // Worker is performing final cleanup before exiting
)

// Pre-allocated error instances to reduce allocation overhead
var (
	errMarshalContent        = errors.New("failed to marshal content")
	errAddToBatch            = errors.New("failed to add to WriteBatch")
	errFlushBatch            = errors.New("failed to flush WriteBatch")
	errWriteOperationTimeout = errors.New("write operation timed out")
)

// NewQueuedWriteBatch creates a new queued write batch system
func NewQueuedWriteBatch(db *badger.DB, opts QueuedWriteBatchOptions) *QueuedWriteBatch {
	qwb := &QueuedWriteBatch{
		writeQueue:   make(chan *WriteTask, opts.QueueSize),
		maxQueueSize: opts.QueueSize,
		batchTimeout: opts.BatchTimeout,
		maxBatchSize: opts.BatchSize,
		stopChan:     make(chan struct{}),
		metrics: QueueMetrics{
			QueueDepth:      0,
			TotalQueued:     0,
			TotalProcessed:  0,
			TotalErrors:     0,
			processingCount: 0,
		},
		db:            db,
		pendingItems:  NewCountedSyncMap(),
		notifications: NewUnifiedNotificationSystem(),
	}

	// Set default error handler if none provided
	if opts.ErrorHandler != nil {
		qwb.errorHandler = opts.ErrorHandler
	} else {
		qwb.errorHandler = qwb.defaultErrorHandler
	}

	// Start single worker goroutine
	qwb.startWorker()

	return qwb
}

// startWorker starts the single worker goroutine that processes the write queue
func (qwb *QueuedWriteBatch) startWorker() {
	qwb.workerWg.Add(1)
	go qwb.worker()
}

// stopWorker gracefully stops the worker goroutine by sending a stop signal
func (qwb *QueuedWriteBatch) stopWorker() {
	select {
	case <-qwb.stopChan:
		return
	default:
		close(qwb.stopChan)
		qwb.workerWg.Wait()
		return
	}
}

// worker processes write tasks from the queue in batches using a state machine.
func (qwb *QueuedWriteBatch) worker() {
	defer qwb.workerWg.Done()

	batch := make([]*WriteTask, 0, qwb.maxBatchSize)
	batchTimer := time.NewTimer(qwb.batchTimeout)
	batchTimer.Stop()

	currentState := workerStateRunning // Initial state

	for {
		switch currentState {
		case workerStateRunning:
			select {
			case task := <-qwb.writeQueue:
				// Add task to current batch
				batch = append(batch, task)

				// Start timer if this is the first item in batch
				if len(batch) == 1 {
					batchTimer.Reset(qwb.batchTimeout)
				}

				// Process when batch reaches max size
				if len(batch) >= qwb.maxBatchSize {
					qwb.processBatch(batch) // worker processes 1 batch
					batch = batch[:0]       // Reset batch
					batchTimer.Stop()
				}

			case <-batchTimer.C:
				// Timeout reached
				if len(batch) > 0 {
					qwb.processBatch(batch) // process partial batch
					batch = batch[:0]       // Reset batch
				}

			case <-qwb.stopChan:
				// Transition to stopping state
				currentState = workerStateStopping

			} // End select for workerStateRunning

		case workerStateStopping:
			// Final processing of any remaining (local) batch if not already done by previous state transitions
			if len(batch) > 0 {
				qwb.processBatch(batch) // process final batch
			}
			batchTimer.Stop()  // Ensure timer is stopped
			goto endWorkerLoop // Exit the outer for loop
		} // End switch (currentState)
	} // End for loop

endWorkerLoop:
	// Worker cleanup: The simpleFlush() function is responsible for draining the queue
	// during shutdown. The worker's job is to process its local batch and exit cleanly.
}

// simpleFlush implements graceful shutdown queue drainage with comprehensive state cleanup.
//
// This function performs a coordinated shutdown of the write queue system, ensuring
// all pending operations are completed before shutdown while maintaining data consistency.
//
// ## Shutdown Sequence:
// 1. **Worker Termination** - Send stop signal to worker and wait for acknowledgment
// 2. **Queue Drainage** - Process any tasks remaining in the write queue channel
// 3. **Pending Task Processing** - Force completion of tasks consumed but not processed
// 4. **Consistency Wait** - Brief delay to ensure database operations complete
//
// ## Data Consistency Guarantees:
// - All queued tasks are either processed or safely discarded
// - Pending items map is cleared only after successful processing
// - Database operations are given time to complete before shutdown
// - No data loss occurs during graceful shutdown
//
// ## Performance Characteristics:
// - **Fast Path**: Returns immediately if no pending items exist
// - **Timeout Protection**: Uses non-blocking operations to prevent hanging
// - **Minimal Delays**: Only adds delays when necessary for consistency
// - **Resource Cleanup**: Ensures proper cleanup of internal state
//
// ## Error Handling:
// - Continues processing even if individual tasks fail
// - Logs errors but doesn't abort shutdown process
// - Returns aggregated error status for monitoring
//
// ## Usage Context:
// - Called during graceful server shutdown (SIGTERM, single Ctrl+C)
// - Used before critical operations requiring queue consistency
// - Invoked by FlushQueue() API for manual queue drainage
//
// Returns error if critical operations fail, but shutdown continues regardless.
func (qwb *QueuedWriteBatch) simpleFlush() error {
    // Fast check if there's anything to flush
    if qwb.pendingItems.IsEmpty() {
        return nil
    }

    // 1. Send signal to worker first
    qwb.stopWorker()

    // 2. Drain any remaining tasks from writeQueue
    // (tasks that worker didn't consume before stopping)
    drainedCount := qwb.drainQueuedTasksNonBlocking()
    if drainedCount > 0 {
        time.Sleep(100 * time.Millisecond)
    }

    // 3. Force process any stuck tasks in pendingItems
    // (tasks that worker consumed but didn't complete before stopping)
    qwb.forceProcessStuckTasks()

    // 4. Brief wait for final database operations to complete
    if drainedCount > 0 {
        time.Sleep(100 * time.Millisecond)
    }

    finalPending := qwb.GetPendingContentCount()
    if finalPending > 0 {
        return fmt.Errorf("simple flush incomplete: %d items still pending after worker stop and drain", finalPending)
    }

    return nil
}

// processBatch processes a batch of write tasks using BadgerDB WriteBatch
// Sequential ordering is guaranteed by single worker processing tasks from queue in order
func (qwb *QueuedWriteBatch) processBatch(tasks []*WriteTask) {
	if len(tasks) == 0 {
		return
	}

	// Create a new WriteBatch for this batch
	writeBatch := qwb.db.NewWriteBatch()
	defer writeBatch.Cancel()

	// Process tasks in queue order - BadgerDB naturally handles overwrites (last write wins)
	// No need for complex deduplication since single worker ensures sequential processing
	for _, task := range tasks {
		// Use pre-marshaled data if available, otherwise marshal now
		var data []byte
		var err error
		if task.MarshaledData != nil {
			data = task.MarshaledData
		} else {
			data, err = json.Marshal(task.Content)
			if err != nil {
				qwb.handleTaskError(task, errMarshalContent)
				continue
			}
		}

		// Add to WriteBatch - BadgerDB handles key overwrites internally
		if err := writeBatch.Set([]byte(task.Content.ID), data); err != nil {
			qwb.handleTaskError(task, errAddToBatch)
			continue
		}
	}

	// Flush the entire batch
	if err := writeBatch.Flush(); err != nil {
		// Handle error for all tasks in batch
		for _, task := range tasks {
			qwb.handleTaskError(task, errFlushBatch)
		}
	} else {
		// Success - notify all tasks and clean up pending items
		processedIDs := make(map[string]bool, len(tasks))

		// Notify all synchronous operations
		for _, task := range tasks {
			if task.ResultChan != nil {
				select {
				case task.ResultChan <- nil:
				default:
					// Channel might be closed or full
				}
			}
			// Track unique content IDs for cleanup
			processedIDs[task.Content.ID] = true
		}

		// Remove from pending items and notify completion for each unique content ID
		// Only decrement metrics for items that were actually in pendingItems
		actuallyProcessedCount := 0
		for contentID := range processedIDs {
			// Use LoadAndDelete to atomically check and remove
			if _, wasPresent := qwb.pendingItems.LoadAndDelete(contentID); wasPresent {
				actuallyProcessedCount++
			}
			qwb.notifications.notifyCompletion(contentID)
		}

		// Update metrics atomically - only decrement for items that were actually pending
		atomic.AddInt64(&qwb.metrics.TotalProcessed, int64(len(tasks)))
		if actuallyProcessedCount > 0 {
			atomic.AddInt64(&qwb.metrics.QueueDepth, -int64(actuallyProcessedCount))
		}

		// Check if all pending items are now complete and notify flush waiters
		qwb.notifications.checkAndNotifyFlushCompletion(qwb.pendingItems)
	}
}

// DoubleQueueCapacity safely doubles the queue capacity by creating a new channel
// and transferring existing tasks. This operation is thread-safe and coordinates
// with the worker goroutine to ensure no tasks are lost.
//
// The approach used here is to create a new channel with double capacity and
// atomically swap it with the old one. The worker goroutine will automatically
// start using the new channel since it reads from qwb.writeQueue.
func (qwb *QueuedWriteBatch) DoubleQueueCapacity() error {
	// Use mutex to ensure only one capacity doubling operation at a time
	qwb.capacityMutex.Lock()
	defer qwb.capacityMutex.Unlock()

	// Check if we're still running - don't resize if shutting down
	if qwb.stopChanClosed() {
		return fmt.Errorf("cannot double queue capacity: system is shutting down")
	}

	// Calculate new capacity (double the current)
	oldCapacity := qwb.maxQueueSize
	newCapacity := oldCapacity * 2

	// Create new channel with doubled capacity
	newQueue := make(chan *WriteTask, newCapacity)

	// Get the current queue length to know how many tasks to transfer
	// We use len() which is safe for channels
	currentQueueLen := len(qwb.writeQueue)

	var transferredCount int // Declare transferredCount here

	// Transfer existing tasks from old queue to new queue
	// This loop will block until all tasks are transferred to the new queue,
	// ensuring no data loss during capacity doubling.
	for i := 0; i < currentQueueLen; i++ {
		task := <-qwb.writeQueue // This will block if the old queue is empty, but we know its length
		newQueue <- task         // This will block if the new queue is full, ensuring transfer
		transferredCount++
	}

	// Atomically replace the old queue with the new one
	// This is the critical section where we swap the channels
	qwb.writeQueue = newQueue
	qwb.maxQueueSize = newCapacity

	// Log the capacity change for monitoring
	// In production, this would use structured logging
	// fmt.Printf("Queue capacity doubled from %d to %d, transferred %d tasks\n",
	//     oldCapacity, newCapacity, transferredCount)

	return nil
}

func (qwb *QueuedWriteBatch) storePendingItem(contentID string, task *WriteTask) {
	qwb.pendingItems.Store(contentID, task)
	atomic.AddInt64(&qwb.metrics.TotalQueued, 1)
	atomic.AddInt64(&qwb.metrics.QueueDepth, 1) // Atomically update queue depth
}

func (qwb *QueuedWriteBatch) removePendingItem(contentID string) {
	qwb.pendingItems.Delete(contentID)
	atomic.AddInt64(&qwb.metrics.QueueDepth, -1) // Atomically update queue depth
}

// QueueWrite queues a write operation asynchronously
func (qwb *QueuedWriteBatch) QueueWrite(content *models.Content) error {
	// Pre-marshal JSON data to avoid repeated marshaling in processBatch
	data, err := json.Marshal(content)
	if err != nil {
		return errMarshalContent
	}

	task := &WriteTask{
		Content:       content,
		MarshaledData: data,
	}

	// Check queue capacity and handle full queue by doubling capacity
	select {
	case qwb.writeQueue <- task:
		// Successfully queued - track pending item (always store the latest task for this ID)
		// Note: This overwrites previous pending tasks for the same ID, but that's intentional
		// since we only need to track that there's a pending write for this ID
		qwb.storePendingItem(content.ID, task)
		return nil
	default:
		// Queue is full - double the capacity instead of returning error
		if err := qwb.DoubleQueueCapacity(); err != nil {
			// If capacity doubling fails, fall back to original error behavior
			currentMaxQueueSize := qwb.getMaxQueueSizeProtected()
			return fmt.Errorf("write queue is full (capacity: %d) and failed to double capacity: %v", currentMaxQueueSize, err)
		}

		// Try to queue again with the new doubled capacity
		select {
		case qwb.writeQueue <- task:
			// Successfully queued after capacity doubling
			qwb.storePendingItem(content.ID, task)
			return nil
		default:
			// This should be very rare - the new queue is also full immediately
			// This could happen under extreme load conditions
			currentMaxQueueSize := qwb.getMaxQueueSizeProtected()
			return fmt.Errorf("write queue is still full even after doubling capacity to %d", currentMaxQueueSize)
		}
	}
}

// QueueWriteSync queues a write operation and waits for completion
func (qwb *QueuedWriteBatch) QueueWriteSync(content *models.Content) error {
	// Pre-marshal JSON data to avoid repeated marshaling in processBatch
	data, err := json.Marshal(content)
	if err != nil {
		return errMarshalContent
	}

	task := &WriteTask{
		Content:       content,
		MarshaledData: data,
		ResultChan:    make(chan error, 1),
	}

	// Queue the task with capacity doubling if needed
	select {
	case qwb.writeQueue <- task:
		// Successfully queued - track pending item
		qwb.storePendingItem(content.ID, task)
	default:
		// Queue is full - double the capacity instead of returning error
		if err := qwb.DoubleQueueCapacity(); err != nil {
			// If capacity doubling fails, fall back to original error behavior
			currentMaxQueueSize := qwb.getMaxQueueSizeProtected()
			return fmt.Errorf("write queue is full (capacity: %d) and failed to double capacity: %v", currentMaxQueueSize, err)
		}

		// Try to queue again with the new doubled capacity
		select {
		case qwb.writeQueue <- task:
			// Successfully queued after capacity doubling
			qwb.storePendingItem(content.ID, task)
		default:
			// This should be very rare - the new queue is also full immediately
			// This could happen under extreme load conditions
			currentMaxQueueSize := qwb.getMaxQueueSizeProtected()
			return fmt.Errorf("write queue is still full even after doubling capacity to %d", currentMaxQueueSize)
		}
	}

	// Wait for result
	select {
	case err := <-task.ResultChan:
		return err
	case <-time.After(30 * time.Second): // Timeout for sync operations
		qwb.removePendingItem(content.ID)
		return errWriteOperationTimeout
	}
}

// WaitForContentWrite waits for a specific content ID to complete writing using event-driven notifications
func (qwb *QueuedWriteBatch) WaitForContentWrite(contentID string, timeout time.Duration) error {
	// Check if content is actually in queue
	if !qwb.IsContentInQueue(contentID) {
		return nil // Not in queue, already completed or never queued
	}

	// Create notification channel for this specific content ID
	notifyChan := make(chan struct{}, 1)

	// Register for completion notification
	qwb.notifications.registerCompletionWaiter(contentID, notifyChan)
	defer qwb.notifications.unregisterCompletionWaiter(contentID, notifyChan)

	// Wait for completion or timeout
	select {
	case <-notifyChan:
		return nil // Write completed
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for content %s write to complete", contentID)
	}
}

// handleTaskError handles errors for individual tasks
func (qwb *QueuedWriteBatch) handleTaskError(task *WriteTask, err error) {
	atomic.AddInt64(&qwb.metrics.TotalErrors, 1)

	// Remove from pending items on error - only decrement if item was actually present
	if _, wasPresent := qwb.pendingItems.LoadAndDelete(task.Content.ID); wasPresent {
		// Update queue depth atomically only if item was actually pending
		atomic.AddInt64(&qwb.metrics.QueueDepth, -1)
	}

	// Notify completion waiters for this specific content ID (even on error)
	qwb.notifications.notifyCompletion(task.Content.ID)

	// Notify synchronous operations
	if task.ResultChan != nil {
		select {
		case task.ResultChan <- err:
		default:
			// Channel might be closed or full
		}
	}

	// Check if all pending items are now complete and notify flush waiters
	qwb.notifications.checkAndNotifyFlushCompletion(qwb.pendingItems)

	// Call custom error handler
	if qwb.errorHandler != nil {
		qwb.errorHandler(task, err)
	}
}

// defaultErrorHandler provides basic error handling
func (qwb *QueuedWriteBatch) defaultErrorHandler(task *WriteTask, err error) {
	// In production, this would use proper structured logging
	// For now, we'll just track the error in metrics
	// fmt.Printf("Write task error for content %s: %v\n", task.Content.ID, err)
}

func (qwb *QueuedWriteBatch) closeQueue() {
	if qwb.writeQueue != nil {
		close(qwb.writeQueue)
	}
}

// Stop gracefully stops the queued write system
func (qwb *QueuedWriteBatch) Stop() {
	qwb.stopOnce.Do(func() {
		qwb.stopWorker()
		qwb.closeQueue()
	})
}

// EmergencyStop immediately terminates the queue worker for catastrophic shutdown scenarios.
//
// This function implements the emergency termination protocol for the write queue worker,
// designed for situations where immediate shutdown is required regardless of data loss.
//
// ## Emergency Termination Protocol:
// - **Immediate Signal**: Sends stop signal to worker without waiting for acknowledgment
// - **No Cleanup**: Skips normal cleanup procedures to minimize shutdown time
// - **No Waiting**: Does not wait for worker to finish current operations
// - **Idempotent**: Safe to call multiple times without side effects
//
// ## Data Preservation Strategy:
// - **Volatile State**: Pending operations remain in memory for potential serialization
// - **Queue Contents**: Write queue channel contents are preserved until serialization
// - **No Database Flush**: Database operations may be incomplete at termination
//
// ## Performance Characteristics:
// - **Target Time**: <100ms execution time
// - **Non-blocking**: Uses select with default to avoid blocking on closed channels
// - **Minimal Operations**: Only performs essential termination signal
//
// ## Usage Context:
// - Called during emergency shutdown (double Ctrl+C, SIGUSR1)
// - Used when immediate termination is required due to system failure
// - Followed by queue state serialization for recovery
//
// ## Recovery Integration:
// - Worker state is preserved for emergency serialization
// - Pending items remain accessible for recovery file generation
// - Queue contents can be extracted after emergency stop
//
// Returns error only for unexpected conditions. Emergency shutdown continues regardless.
func (qwb *QueuedWriteBatch) EmergencyStop() error {
	// Send stop signal to worker immediately
	select {
	case <-qwb.stopChan:
		// Already stopped
		return nil
	default:
		close(qwb.stopChan)
	}

	// Don't wait for worker to finish - this is emergency stop
	return nil
}

// IsRunning returns whether the queued write system is currently running
// This checks if the stop channel is still open (workers are active)
func (qwb *QueuedWriteBatch) IsRunning() bool {
	return !qwb.stopChanClosed()
}

func (qwb *QueuedWriteBatch) stopChanClosed() bool {
	select {
	case <-qwb.stopChan:
		return true
	default:
		return false
	}
}

// GetMetrics returns current queue metrics with atomic reads for thread-safety
func (qwb *QueuedWriteBatch) GetMetrics() QueueMetrics {
	// All fields are read atomically - no mutex needed
	return QueueMetrics{
		QueueDepth:      atomic.LoadInt64(&qwb.metrics.QueueDepth),
		TotalQueued:     atomic.LoadInt64(&qwb.metrics.TotalQueued),
		TotalProcessed:  atomic.LoadInt64(&qwb.metrics.TotalProcessed),
		TotalErrors:     atomic.LoadInt64(&qwb.metrics.TotalErrors),
		processingCount: atomic.LoadInt64(&qwb.metrics.processingCount),
	}
}

// IsContentInQueue checks if content with the given ID is currently in the queue
func (qwb *QueuedWriteBatch) IsContentInQueue(contentID string) bool {
	_, exists := qwb.pendingItems.Load(contentID)
	return exists
}

// GetPendingContent retrieves the pending WriteTask for the given content ID
// Returns the task and true if found, nil and false if not found
func (qwb *QueuedWriteBatch) GetPendingContent(contentID string) (*WriteTask, bool) {
	if task, exists := qwb.pendingItems.Load(contentID); exists {
		return task.(*WriteTask), true
	}
	return nil, false
}

// GetPendingContentIDs returns a slice of all content IDs currently pending in the queue
func (qwb *QueuedWriteBatch) GetPendingContentIDs() []string {
	// Pre-allocate slice with exact capacity to avoid reallocations
	// Use actual count from CountedSyncMap for optimal memory usage
	count := qwb.pendingItems.Count()
	if count == 0 {
		return nil // Return nil for empty case to avoid allocation
	}

	ids := make([]string, 0, count) // Exact capacity allocation
	qwb.pendingItems.Range(func(key string, value interface{}) bool {
		ids = append(ids, key)
		return true
	})
	return ids
}

// GetPendingContentCount returns the number of items currently pending in the queue
// Uses atomic counter for O(1) performance instead of O(n) sync.Map iteration
func (qwb *QueuedWriteBatch) GetPendingContentCount() int {
	return qwb.pendingItems.Count()
}

// drainQueuedTasksNonBlocking drains and processes tasks currently in writeQueue non-blockingly.
func (qwb *QueuedWriteBatch) drainQueuedTasksNonBlocking() int {

	debug := true

	var processedThisCall int
	batch := make([]*WriteTask, 0, qwb.maxBatchSize)

	// Non-blocking drain of writeQueue channel
	for {
		select {
		case task := <-qwb.writeQueue:
			batch = append(batch, task)
			processedThisCall++
			if len(batch) >= qwb.maxBatchSize {
				if debug {
					fmt.Println("drainQueuedTasksNonBlocking() Drain 1 batch.")
				}
				qwb.processBatch(batch) // drain 1 batch
				batch = batch[:0]       // Reset batch
			}
		default:
			goto processExtracted
		}
	}

processExtracted:
	if debug {
		fmt.Println("drainQueuedTasksNonBlocking() Process extracted tasks.")
	}
	// Process any remaining tasks that didn't form a full batch
	if len(batch) > 0 {
		if debug {
			fmt.Println("drainQueuedTasksNonBlocking() Process last batch.")
		}
		qwb.processBatch(batch) // drain last batch
	}

	// Note: For active flush scenarios, stuck tasks in pendingItems are handled
	// by the notification system and don't need forced processing here.
	// Shutdown scenarios use simpleFlush() which handles stuck tasks separately.

	return processedThisCall
}

// forceProcessStuckTasks processes tasks that are stuck in pendingItems during shutdown
// This handles the case where worker pulled tasks from writeQueue but stopped before completing them
func (qwb *QueuedWriteBatch) forceProcessStuckTasks() {
	debug := true

	// Collect all tasks from pendingItems
	var stuckTasks []*WriteTask
	qwb.pendingItems.Range(func(key string, value interface{}) bool {
		if task, ok := value.(*WriteTask); ok {
			stuckTasks = append(stuckTasks, task)
		}
		return true
	})

	if len(stuckTasks) == 0 {
		return // No stuck tasks to process
	}

	if debug {
		fmt.Printf("Force processing %d stuck tasks\n", len(stuckTasks))
	}

	// Process stuck tasks in batches to complete their persistence
	for i := 0; i < len(stuckTasks); i += qwb.maxBatchSize {
		end := i + qwb.maxBatchSize
		if end > len(stuckTasks) {
			end = len(stuckTasks)
		}
		batch := stuckTasks[i:end]
		qwb.processBatch(batch)

		if debug {
			fmt.Printf("Force processed batch of %d stuck tasks\n", len(batch))
		}
	}

	// Ensure flush completion notification is triggered after force processing
	qwb.notifications.checkAndNotifyFlushCompletion(qwb.pendingItems)

	if debug {
		remaining := qwb.GetPendingContentCount()
		fmt.Printf("After force processing: %d items still pending\n", remaining)
	}
}

// SerializePendingTasks extracts all pending tasks and returns them as JSON for emergency recovery
func (qwb *QueuedWriteBatch) SerializePendingTasks() ([]byte, error) {
	var tasks []*WriteTask

	// Extract all pending tasks
	qwb.pendingItems.Range(func(key string, value interface{}) bool {
		if task, ok := value.(*WriteTask); ok {
			tasks = append(tasks, task)
		}
		return true
	})

	// Convert to serializable format (exclude channels)
	type SerializableTask struct {
		Content       *models.Content `json:"content"`
		MarshaledData []byte          `json:"marshaled_data"`
	}

	serializableTasks := make([]SerializableTask, len(tasks))
	for i, task := range tasks {
		serializableTasks[i] = SerializableTask{
			Content:       task.Content,
			MarshaledData: task.MarshaledData,
		}
	}

	return json.Marshal(serializableTasks)
}

// SerializeWriteQueue drains the write queue and returns tasks as JSON for emergency recovery
func (qwb *QueuedWriteBatch) SerializeWriteQueue() ([]byte, error) {
	var tasks []*WriteTask

	// Drain all tasks from write queue non-blocking
	for {
		select {
		case task := <-qwb.writeQueue:
			tasks = append(tasks, task)
		default:
			// No more tasks in queue
			goto done
		}
	}

done:
	// Convert to serializable format (exclude channels)
	type SerializableTask struct {
		Content       *models.Content `json:"content"`
		MarshaledData []byte          `json:"marshaled_data"`
	}

	serializableTasks := make([]SerializableTask, len(tasks))
	for i, task := range tasks {
		serializableTasks[i] = SerializableTask{
			Content:       task.Content,
			MarshaledData: task.MarshaledData,
		}
	}

	return json.Marshal(serializableTasks)
}

// GetHealthStatus returns the health status of the queue system
func (qwb *QueuedWriteBatch) GetHealthStatus() HealthStatus {
	if qwb.stopChanClosed() {
		return HealthStatusUnhealthy
	}

	// Simple check: if queue is very full, it's degraded
	metrics := qwb.GetMetrics()

	currentMaxQueueSize := qwb.getMaxQueueSizeProtected()

	if float64(metrics.QueueDepth)/float64(currentMaxQueueSize) > 0.8 {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}

// GetMaxQueueSize returns the current maximum queue size
// This is useful for monitoring and debugging capacity doubling behavior
func (qwb *QueuedWriteBatch) GetMaxQueueSize() int {
	qwb.capacityMutex.Lock()
	defer qwb.capacityMutex.Unlock()
	return qwb.maxQueueSize
}

// GetActualQueueDepth returns the actual number of items currently in the write queue channel
// This provides the real queue state, unlike the broken QueueDepth metric
func (qwb *QueuedWriteBatch) GetActualQueueDepth() int {
	return len(qwb.writeQueue)
}

// getMaxQueueSizeProtected returns the current maximum queue size with mutex protection
func (qwb *QueuedWriteBatch) getMaxQueueSizeProtected() int {
	qwb.capacityMutex.Lock()
	defer qwb.capacityMutex.Unlock()
	return qwb.maxQueueSize
}
