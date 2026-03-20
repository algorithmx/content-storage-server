package storage

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"content-storage-server/pkg/models"
)

func TestCountedSyncMapRaceCondition(t *testing.T) {
	csm := NewCountedSyncMap()

	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 1000

	var finalCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d", j%(opsPerGoroutine/10))
				_ = key
				csm.Store(key, "value")
			}
		}()
	}

	wg.Wait()

	actualCount := csm.Count()
	expectedCount := int64(opsPerGoroutine / 10)

	atomic.StoreInt64(&finalCount, int64(actualCount))

	t.Logf("CountedSyncMap final count: %d, expected: %d", actualCount, expectedCount)

	if actualCount < 0 {
		t.Errorf("CRITICAL: Count is negative! Counter desynced. Got: %d", actualCount)
	}
}

func TestCountedSyncMapStoreConcurrent(t *testing.T) {
	csm := NewCountedSyncMap()

	var wg sync.WaitGroup
	key := "shared-key"

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				csm.Store(key, "value")
			}
		}()
	}

	wg.Wait()

	count := csm.Count()
	if count != 1 {
		t.Errorf("Expected count 1 after concurrent Store on same key, got %d", count)
	}

	if count < 0 {
		t.Errorf("CRITICAL: Count went negative: %d", count)
	}
}

func TestCountedSyncMapStoreDeleteRace(t *testing.T) {
	csm := NewCountedSyncMap()

	var wg sync.WaitGroup
	numOperations := 50000

	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n%50)
			if n%2 == 0 {
				csm.Store(key, "value")
			} else {
				csm.Delete(key)
			}
		}(i)
	}

	wg.Wait()

	count := csm.Count()
	t.Logf("Count after Store/Delete race: %d (expected 0-50)", count)

	if count < 0 {
		t.Errorf("CRITICAL: Count went NEGATIVE: %d - race condition confirmed!", count)
	}
}

func TestQueueDepthMetricAccuracy(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-queuedepth-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	queueSize := 50
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    queueSize,
		BatchSize:    10,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	expires := time.Now().Add(1 * time.Hour)
	numItems := 30

	for i := 0; i < numItems; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("depth-test-%d", i),
			Type:      "text/plain",
			Data:      "test data",
			ExpiresAt: &expires,
		}
		if err := qwb.QueueWrite(content); err != nil {
			t.Fatalf("Failed to queue write: %v", err)
		}
	}

	actualDepth := len(qwb.writeQueue)
	metrics := qwb.GetMetrics()
	reportedDepth := metrics.QueueDepth

	t.Logf("Actual queue depth (len): %d", actualDepth)
	t.Logf("Reported QueueDepth metric: %d", reportedDepth)
	t.Logf("Pending items count: %d", qwb.GetPendingContentCount())

	if actualDepth != int(reportedDepth) {
		t.Logf("WARNING: QueueDepth metric mismatch! Actual: %d, Reported: %d", actualDepth, reportedDepth)
	}

	time.Sleep(200 * time.Millisecond)

	processedMetrics := qwb.GetMetrics()
	t.Logf("After processing - QueueDepth: %d, TotalProcessed: %d",
		processedMetrics.QueueDepth, processedMetrics.TotalProcessed)

	if processedMetrics.QueueDepth < 0 {
		t.Errorf("CRITICAL: QueueDepth went negative! Value: %d", processedMetrics.QueueDepth)
	}
}

func TestQueueDepthAfterCapacityDoubling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-depth-double-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	initialSize := 10
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 50 * time.Millisecond,
	})
	defer qwb.Stop()

	expires := time.Now().Add(1 * time.Hour)

	for i := 0; i < 15; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("double-test-%d", i),
			Type:      "text/plain",
			Data:      "test data",
			ExpiresAt: &expires,
		}
		_ = qwb.QueueWrite(content)
	}

	actualDepth := len(qwb.writeQueue)
	metrics := qwb.GetMetrics()

	t.Logf("After capacity doubling - Actual: %d, Metric: %d, Capacity: %d",
		actualDepth, metrics.QueueDepth, qwb.GetMaxQueueSize())

	if actualDepth != int(metrics.QueueDepth) {
		t.Logf("WARNING: Depth mismatch after doubling! Actual: %d, Metric: %d",
			actualDepth, metrics.QueueDepth)
	}
}

func TestCapacityDoublingWithConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "badger-cap-double-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	initialSize := 50
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    20,
		BatchTimeout: 200 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(5)
	maxCapacity := initialSize * 5
	expires := time.Now().Add(1 * time.Hour)

	var writeErrors int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				content := &models.Content{
					ID:        fmt.Sprintf("concurrent-%d-%d", i, j),
					Type:      "text/plain",
					Data:      "test data",
					ExpiresAt: &expires,
				}
				if err := qwb.QueueWrite(content); err != nil {
					atomic.AddInt32(&writeErrors, 1)
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		for i := 0; i < 3; i++ {
			qwb.DoubleQueueCapacity()
			time.Sleep(20 * time.Millisecond)
		}
	}()

	wg.Wait()

	finalCapacity := qwb.GetMaxQueueSize()
	t.Logf("Final capacity: %d, max allowed: %d, write errors: %d",
		finalCapacity, maxCapacity, atomic.LoadInt32(&writeErrors))

	if finalCapacity > maxCapacity {
		t.Errorf("CRITICAL: Capacity exceeded max! Got: %d, Max: %d", finalCapacity, maxCapacity)
	}
}

func TestSyncMapVsRWMutexPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark in short mode")
	}

	const numItems = 10000
	const numOps = 100000

	type RWMutexMap struct {
		mu   sync.RWMutex
		data map[string]*models.AccessTracker
	}
	rwMap := &RWMutexMap{
		data: make(map[string]*models.AccessTracker),
	}

	syncMap := &sync.Map{}

	for i := 0; i < numItems; i++ {
		key := fmt.Sprintf("item-%d", i)
		tracker := &models.AccessTracker{ID: key}
		rwMap.data[key] = tracker
		syncMap.Store(key, tracker)
	}

	runtime.GC()

	start := time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("item-%d", i%numItems)
		rwMap.mu.RLock()
		_, _ = rwMap.data[key]
		rwMap.mu.RUnlock()
	}
	rwMutexReadTime := time.Since(start)

	runtime.GC()

	start = time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("item-%d", i%numItems)
		_, _ = syncMap.Load(key)
	}
	syncMapReadTime := time.Since(start)

	runtime.GC()

	start = time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("item-%d", i%numItems)
		rwMap.mu.Lock()
		rwMap.data[key] = &models.AccessTracker{ID: key}
		rwMap.mu.Unlock()
	}
	rwMutexWriteTime := time.Since(start)

	runtime.GC()

	start = time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("item-%d", i%numItems)
		syncMap.Store(key, &models.AccessTracker{ID: key})
	}
	syncMapWriteTime := time.Since(start)

	t.Logf("Performance comparison (%d ops on %d items):", numOps, numItems)
	t.Logf("  RWMutex Read:  %v", rwMutexReadTime)
	t.Logf("  sync.Map Read:  %v", syncMapReadTime)
	t.Logf("  RWMutex Write: %v", rwMutexWriteTime)
	t.Logf("  sync.Map Write: %v", syncMapWriteTime)

	if syncMapWriteTime > rwMutexWriteTime*2 {
		t.Logf("WARNING: sync.Map write is significantly slower than RWMutex!")
	}
}

func TestAccessManagerWriteHeavyPattern(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	am := NewAccessManager()

	const numWriters = 50
	const opsPerWriter = 10000

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerWriter; j++ {
				key := fmt.Sprintf("content-%d", j%100)
				_ = am.IncrementAccess(key)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	trackedCount := len(am.GetAllAccessCounts())

	t.Logf("AccessManager write-heavy test: %d writers x %d ops in %v",
		numWriters, opsPerWriter, elapsed)
	t.Logf("  Tracked items: %d", trackedCount)
	t.Logf("  Ops/sec: %.2f", float64(numWriters*opsPerWriter)/elapsed.Seconds())

	if trackedCount < 50 || trackedCount > 150 {
		t.Logf("WARNING: Unexpected tracked count: %d (expected ~100)", trackedCount)
	}
}

func TestCountedSyncMapLoadAndDeleteRace(t *testing.T) {
	csm := NewCountedSyncMap()

	csm.Store("key1", "value1")
	csm.Store("key2", "value2")

	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			csm.Load("key1")
		}()
	}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			csm.LoadAndDelete("key1")
		}()
	}

	wg.Wait()

	count := csm.Count()
	t.Logf("Count after concurrent Load and LoadAndDelete: %d", count)

	if count < 0 {
		t.Errorf("CRITICAL: Count went negative: %d", count)
	}
}

func TestPendingItemsConsistency(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-pending-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	queueSize := 100
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    queueSize,
		BatchSize:    20,
		BatchTimeout: 50 * time.Millisecond,
	})
	defer qwb.Stop()

	expires := time.Now().Add(1 * time.Hour)

	for i := 0; i < 50; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("pending-test-%d", i),
			Type:      "text/plain",
			Data:      "test data",
			ExpiresAt: &expires,
		}
		_ = qwb.QueueWrite(content)
	}

	pendingCount := qwb.GetPendingContentCount()
	metrics := qwb.GetMetrics()
	actualQueueDepth := len(qwb.writeQueue)

	t.Logf("Pending count: %d, QueueDepth metric: %d, Actual queue: %d",
		pendingCount, metrics.QueueDepth, actualQueueDepth)

	time.Sleep(100 * time.Millisecond)

	processedMetrics := qwb.GetMetrics()
	t.Logf("After processing - Pending: %d, QueueDepth: %d",
		qwb.GetPendingContentCount(), processedMetrics.QueueDepth)

	if processedMetrics.QueueDepth < 0 {
		t.Errorf("CRITICAL: QueueDepth negative after processing: %d", processedMetrics.QueueDepth)
	}

	actualAfter := len(qwb.writeQueue)
	if actualAfter != int(processedMetrics.QueueDepth) {
		t.Logf("WARNING: Post-processing mismatch! Actual: %d, Metric: %d",
			actualAfter, processedMetrics.QueueDepth)
	}
}

func TestConcurrentStoreAndDelete(t *testing.T) {
	csm := NewCountedSyncMap()

	var wg sync.WaitGroup
	numOps := 10000

	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n%100)
			if n%3 == 0 {
				csm.Delete(key)
			} else {
				csm.Store(key, "value")
			}
		}(i)
	}

	wg.Wait()

	count := csm.Count()
	t.Logf("Final count after concurrent Store/Delete: %d (expected ~100)", count)

	if count < 0 {
		t.Errorf("CRITICAL: Count is negative: %d", count)
	}
}

func TestHealthStatusWithInaccurateDepth(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-health-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	initialSize := 10
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 50 * time.Millisecond,
	})
	defer qwb.Stop()

	expires := time.Now().Add(1 * time.Hour)

	for i := 0; i < 15; i++ {
		content := &models.Content{
			ID:        fmt.Sprintf("health-test-%d", i),
			Type:      "text/plain",
			Data:      "test data",
			ExpiresAt: &expires,
		}
		_ = qwb.QueueWrite(content)
	}

	healthStatus := qwb.GetHealthStatus()
	metrics := qwb.GetMetrics()
	actualDepth := len(qwb.writeQueue)
	maxSize := qwb.GetMaxQueueSize()

	t.Logf("Health check - Status: %s", healthStatus)
	t.Logf("  Actual depth: %d, Metric: %d, Max size: %d", actualDepth, metrics.QueueDepth, maxSize)
	t.Logf("  Actual utilization: %.1f%%", float64(actualDepth)/float64(maxSize)*100)
	t.Logf("  Metric utilization: %.1f%%", float64(metrics.QueueDepth)/float64(maxSize)*100)

	actualUtil := float64(actualDepth) / float64(maxSize)
	metricUtil := float64(metrics.QueueDepth) / float64(maxSize)

	if actualUtil > 0.8 && healthStatus == HealthStatusHealthy {
		t.Logf("WARNING: Health status might be inaccurate - actual utilization is %.1f%% but status is %s",
			actualUtil*100, healthStatus)
	}

	if metricUtil > 0.8 && actualUtil < 0.5 {
		t.Errorf("CRITICAL: Health check using wrong metric! Actual: %.1f%%, Metric-based: %.1f%%",
			actualUtil*100, metricUtil*100)
	}
}
