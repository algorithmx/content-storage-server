# Performance Issues - Analysis & Fix Plan

## Summary

This document outlines performance issues found in the content-storage-server codebase, based on code review and testing.

---

## Issues Identified

### ✅ FIXED: Issue #3 - AccessManager sync.Map Performance

**Status:** FIXED

**Problem:** 
- AccessManager used `sync.Map` for storing access trackers
- `sync.Map` is optimized for read-heavy (90%+ reads) or "write once, read many" patterns
- AccessManager is **write-heavy** - `IncrementAccess()` is called on every content read
- Benchmark showed `sync.Map` Write was ~2x slower than RWMutex + map

**Solution Applied:**
- Replaced `sync.Map` with `map[string]*AccessTracker + sync.RWMutex`
- Write operations use `mu.Lock()`, read operations use `mu.RLock()`

**Performance Improvement:**
- Before: sync.Map Write: ~18-21ms
- After: RWMutex Write: ~10-12ms
- **~2x faster for writes**

**Files Changed:**
- `pkg/storage/access_manager.go`

---

### 🔴 Issues Remaining

#### Issue #1: QueueDepth Metric Inaccuracy

**Status:** NOT YET FIXED

**Problem:**
- `QueueDepth` metric tracks "total items ever queued" not "current queue depth"
- When items are processed, QueueDepth is decremented
- After capacity doubling, the metric can become inaccurate
- Tests show: Actual: 20, Metric: 30 mismatch

**Location:** `pkg/storage/queue.go`

**Impact:** 
- Monitoring/metrics show wrong values
- Health checks may trigger incorrectly

**Test Coverage:** `TestQueueDepthMetricAccuracy`, `TestQueueDepthAfterCapacityDoubling`

---

#### Issue #2: Capacity Doubling Race Condition

**Status:** ⚠️ CONFIRMED - NOT YET FIXED

**Problem:**
- Data race between `DoubleQueueCapacity()` and `worker()` on `writeQueue` channel
- Race detector confirms: Write at queue.go:393, Read at queue.go:152
- Also race between `DoubleQueueCapacity()` and `QueueWrite()` at queue.go:430

**Location:** `pkg/storage/queue.go:378-393`

**Impact:**
- Potential deadlocks under high concurrency
- Data corruption in edge cases

**Test Coverage:** 
- `TestCapacityDoublingDataRaceBetweenWorkerAndDoubler` ✅ CONFIRMS RACE
- `TestQueueWriteDuringCapacityDoubling` ✅ CONFIRMS RACE (2 races found)

**Race Locations:**
```
1. Write: queue.go:393  (qwb.writeQueue = newQueue)
   Read:  queue.go:152  (task := <-qwb.writeQueue)

2. Write: queue.go:393  (qwb.writeQueue = newQueue)  
   Read:  queue.go:430  (qwb.writeQueue <- task)
```

---

#### Issue #4: Health Status Using Wrong Metric

**Status:** NOT YET FIXED

**Problem:**
- Health check uses `QueueDepth` metric instead of actual `len(writeQueue)`
- Can show 150% utilization when actual is 100%

**Location:** `pkg/storage/queue.go:874` (GetHealthStatus)

**Impact:**
- Incorrect health monitoring
- May cause false alerts

**Test Coverage:** `TestHealthStatusWithInaccurateDepth`

---

## Test Files Created

| File | Purpose |
|------|---------|
| `pkg/storage/performance_critical_test.go` | Tests for critical performance issues |

### Test Commands

```bash
# Run all critical performance tests
go test -v -race -run "TestCountedSyncMap|TestQueueDepth|TestSyncMapVs|TestHealthStatus" ./pkg/storage/...

# Run specific test
go test -v -run "TestAccessManagerWriteHeavyPattern" ./pkg/storage/...
```

---

## Recommendations

### Priority 1 (Critical - Data Races)
1. Fix Issue #2: Capacity doubling race condition

### Priority 2 (High - Correctness)
2. Fix Issue #1: QueueDepth metric accuracy  
3. Fix Issue #4: Health status using wrong metric

### Priority 3 (Optimization)
- Consider implementing batch access count updates
- Consider worker pool for write processing (currently single worker)

---

## Verification

After each fix, run:

```bash
# Run with race detector
go test -race -run "Test.*Critical|TestQueueDepth|TestCapacityDoubling" ./pkg/storage/...

# Run all storage tests
go test -v ./pkg/storage/...
```

---

## Notes

- Issue #3 fix verified: AccessManager now ~2x faster for writes
- All existing tests still pass after the fix
- The fix maintains the same API/behavior, just changes internal implementation