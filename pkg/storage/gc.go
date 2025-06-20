package storage

import (
	"sync"
	"time"
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
)

// GCMetrics tracks essential garbage collection metrics
type GCMetrics struct {
	TotalRuns           int64         // Total number of GC runs
	SuccessfulRuns      int64         // Number of successful GC runs
	FailedRuns          int64         // Number of failed GC runs
	LastRunTime         time.Time     // Time of last GC run
	LastRunDuration     time.Duration // Duration of last GC run
	SpaceReclaimed      int64         // Total space reclaimed (bytes)
	ExpiredItemsRemoved int64         // Total expired content items removed
	mu                  sync.RWMutex  // Mutex for thread-safe access
}

// GarbageCollector manages the garbage collection process for BadgerDB
type GarbageCollector struct {
	db               *badger.DB
	interval         time.Duration
	stopChan         chan struct{}
	isRunning        bool
	wg               sync.WaitGroup  // Wait for goroutine to finish
	mu               sync.RWMutex
	stopOnce         sync.Once       // Ensure stop is called only once
	lastGCTime       time.Time
	gcThreshold      float64 // Minimum ratio of reclaimable space to trigger GC
	adaptiveSchedule bool    // Whether to use adaptive scheduling
	lowTrafficHours  []int   // Hours considered low traffic (0-23)
	metrics          *GCMetrics
	storage          *BadgerStorage // Reference to storage for access manager cleanup
}

// NewGarbageCollector creates a new garbage collector for BadgerDB with optimized defaults
func NewGarbageCollector(db *badger.DB) *GarbageCollector {
	return &GarbageCollector{
		db:               db,
		interval:         5 * time.Minute, // More frequent GC for better performance
		stopChan:         make(chan struct{}),
		isRunning:        false,
		gcThreshold:      0.01,                 // Extremely aggressive cleanup threshold
		adaptiveSchedule: false,                // Disable adaptive scheduling for consistent performance
		lowTrafficHours:  []int{2, 3, 4, 5, 6}, // Low traffic window (not used when adaptive is disabled)
		metrics:          &GCMetrics{},
		storage:          nil, // Will be set by SetStorage method
	}
}

// SetStorage sets the storage reference for access manager cleanup
func (gc *GarbageCollector) SetStorage(storage *BadgerStorage) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.storage = storage
}

// RunGC performs a single garbage collection operation with metrics tracking
func (gc *GarbageCollector) RunGC() error {
	startTime := time.Now()

	// Update metrics - increment total runs
	gc.metrics.mu.Lock()
	gc.metrics.TotalRuns++
	gc.metrics.mu.Unlock()

	// Get database size before GC for space reclamation calculation
	lsmSizeBefore, vlogSizeBefore := gc.db.Size()
	totalSizeBefore := lsmSizeBefore + vlogSizeBefore

	// Run BadgerDB value log GC with configured threshold
	err := gc.db.RunValueLogGC(gc.gcThreshold)

	// Get database size after GC for space reclamation calculation
	lsmSizeAfter, vlogSizeAfter := gc.db.Size()
	totalSizeAfter := lsmSizeAfter + vlogSizeAfter

	duration := time.Since(startTime)
	spaceReclaimed := totalSizeBefore - totalSizeAfter

	// Update metrics based on result
	gc.metrics.mu.Lock()
	gc.metrics.LastRunTime = startTime
	gc.metrics.LastRunDuration = duration

	// Only count positive space reclamation
	if spaceReclaimed > 0 {
		gc.metrics.SpaceReclaimed += spaceReclaimed
	}

	if err != nil && err != badger.ErrNoRewrite {
		gc.metrics.FailedRuns++
	} else {
		gc.metrics.SuccessfulRuns++
		gc.lastGCTime = startTime
	}
	gc.metrics.mu.Unlock()

	// Perform access manager cleanup if needed (memory leak prevention)
	if gc.storage != nil && gc.storage.accessManager != nil && gc.storage.accessManager.ShouldRunCleanup() {
		if contentIDs, cleanupErr := gc.storage.GetAllContentIDs(); cleanupErr == nil {
			removedCount := gc.storage.accessManager.CleanupExpiredTrackers(contentIDs)
			if removedCount > 0 {
				// Track cleanup in metrics for monitoring
				gc.metrics.mu.Lock()
				gc.metrics.ExpiredItemsRemoved += int64(removedCount)
				gc.metrics.mu.Unlock()
			}
		}
	}

	// Return nil for ErrNoRewrite as it's not a real error
	if err == badger.ErrNoRewrite {
		return nil
	}

	return err
}

// SetInterval changes the GC interval
func (gc *GarbageCollector) SetInterval(interval time.Duration) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	// Update interval
	gc.interval = interval

	// Restart GC loop if it's already running
	if gc.isRunning {
		gc.stopInternal()
		gc.startInternal()
	}
}

// Start begins the garbage collection loop
func (gc *GarbageCollector) Start() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.startInternal()
}

// startInternal starts the GC without acquiring the lock (internal use only)
func (gc *GarbageCollector) startInternal() {
	if gc.isRunning {
		return // Already running
	}

	// Create new stop channel only when starting
	gc.stopChan = make(chan struct{})
	gc.isRunning = true

	// Start GC loop with WaitGroup
	gc.wg.Add(1)
	go gc.gcLoop()
}

// Stop halts the garbage collection loop
func (gc *GarbageCollector) Stop() {
	gc.stopOnce.Do(func() {
		gc.mu.Lock()
		defer gc.mu.Unlock()
		gc.stopInternal()
	})
}

// stopInternal stops the GC without acquiring the lock (internal use only)
func (gc *GarbageCollector) stopInternal() {
	if !gc.isRunning {
		return // Not running
	}

	// Close stop channel to signal goroutine to exit
	close(gc.stopChan)

	// Wait for goroutine to finish (this is safe since we're already holding the lock)
	gc.wg.Wait()

	gc.isRunning = false
}

// IsRunning returns whether the garbage collector is currently running
func (gc *GarbageCollector) IsRunning() bool {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return gc.isRunning
}

// gcLoop is the main garbage collection loop with adaptive scheduling
func (gc *GarbageCollector) gcLoop() {
	defer gc.wg.Done()

	ticker := time.NewTicker(gc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if we should run GC now based on adaptive scheduling
			if gc.shouldRunGC() {
				err := gc.RunGC()
				if err != nil && err != badger.ErrNoRewrite {
					// Log error but continue - in production use proper logging
					fmt.Printf("GC error: %v\n", err)
					return
				}
			}
		case <-gc.stopChan:
			return
		}
	}
}

// shouldRunGC determines if GC should run based on adaptive scheduling
func (gc *GarbageCollector) shouldRunGC() bool {
	if !gc.adaptiveSchedule {
		return true // Always run if adaptive scheduling is disabled
	}

	now := time.Now()
	currentHour := now.Hour()

	// Check if current time is in low traffic hours
	isLowTraffic := false
	for _, hour := range gc.lowTrafficHours {
		if currentHour == hour {
			isLowTraffic = true
			break
		}
	}

	// If it's low traffic time, always run
	if isLowTraffic {
		return true
	}

	// During high traffic, be more conservative with GC
	timeSinceLastGC := now.Sub(gc.lastGCTime)

	// If GC has never run (lastGCTime is zero), always run it
	if gc.lastGCTime.IsZero() {
		return true
	}

	// Run GC if we haven't run it for more than the normal interval
	// During high traffic, stick to the configured interval
	return timeSinceLastGC >= gc.interval
}

// SetGCThreshold sets the minimum ratio of reclaimable space to trigger GC
func (gc *GarbageCollector) SetGCThreshold(threshold float64) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.gcThreshold = threshold
}

// SetAdaptiveSchedule enables or disables adaptive GC scheduling
func (gc *GarbageCollector) SetAdaptiveSchedule(enabled bool) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.adaptiveSchedule = enabled
}

// SetLowTrafficHours sets the hours considered low traffic for GC scheduling
func (gc *GarbageCollector) SetLowTrafficHours(hours []int) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.lowTrafficHours = make([]int, len(hours))
	copy(gc.lowTrafficHours, hours)
}

// GetMetrics returns a copy of the current GC metrics
func (gc *GarbageCollector) GetMetrics() GCMetrics {
	gc.metrics.mu.RLock()
	defer gc.metrics.mu.RUnlock()

	// Return a copy to avoid race conditions
	return GCMetrics{
		TotalRuns:           gc.metrics.TotalRuns,
		SuccessfulRuns:      gc.metrics.SuccessfulRuns,
		FailedRuns:          gc.metrics.FailedRuns,
		LastRunTime:         gc.metrics.LastRunTime,
		LastRunDuration:     gc.metrics.LastRunDuration,
		SpaceReclaimed:      gc.metrics.SpaceReclaimed,
		ExpiredItemsRemoved: gc.metrics.ExpiredItemsRemoved,
	}
}

// ResetMetrics resets all GC metrics
func (gc *GarbageCollector) ResetMetrics() {
	gc.metrics.mu.Lock()
	defer gc.metrics.mu.Unlock()

	gc.metrics.TotalRuns = 0
	gc.metrics.SuccessfulRuns = 0
	gc.metrics.FailedRuns = 0
	gc.metrics.LastRunTime = time.Time{}
	gc.metrics.LastRunDuration = 0
	gc.metrics.SpaceReclaimed = 0
	gc.metrics.ExpiredItemsRemoved = 0
}

// GetChannelStats returns channel and goroutine statistics for monitoring
// This is primarily useful for debugging and operational monitoring
func (gc *GarbageCollector) GetChannelStats() map[string]interface{} {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	return map[string]interface{}{
		"stop_channel_initialized": gc.stopChan != nil,
		"is_running":               gc.isRunning,
		"component_type":           "garbage_collector",
		"last_gc_time":             gc.lastGCTime,
	}
}
