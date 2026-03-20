# Issue #2: Capacity Doubling Race Condition - Investigation Report

## Executive Summary

A data race was discovered in the `QueuedWriteBatch` implementation where the `DoubleQueueCapacity()` function modifies the `writeQueue` channel while other goroutines (worker and QueueWrite) are concurrently reading/writing to the same channel. This race condition was confirmed using Go's race detector.

---

## Issue Details

### Location
- **File:** `pkg/storage/queue.go`
- **Functions:** `DoubleQueueCapacity()`, `worker()`, `QueueWrite()`

### Affected Code

```go
// queue.go:385-393 - The problematic section
for i := 0; i < currentQueueLen; i++ {
    task := <-qwb.writeQueue  // Read from old queue
    newQueue <- task         // Write to new queue
}
qwb.writeQueue = newQueue     // ATOMIC SWAP - but race here!
qwb.maxQueueSize = newCapacity
```

---

## Race Condition Analysis

### Race #1: DoubleQueueCapacity vs Worker

| Aspect | Details |
|--------|---------|
| **Write Location** | `queue.go:393` - `qwb.writeQueue = newQueue` |
| **Read Location** | `queue.go:152` - `task := <-qwb.writeQueue` |
| **Description** | Worker reads from writeQueue while DoubleQueueCapacity is swapping it |

### Race #2: DoubleQueueCapacity vs QueueWrite

| Aspect | Details |
|--------|---------|
| **Write Location** | `queue.go:393` - `qwb.writeQueue = newQueue` |
| **Read Location** | `queue.go:430` - `qwb.writeQueue <- task` (select default case) |
| **Description** | QueueWrite attempts to write to queue while it's being swapped |

---

## Test Evidence

### Test 1: TestCapacityDoublingDataRaceBetweenWorkerAndDoubler

```
==================
WARNING: DATA RACE
Write at 0x00c0001e64d0 by goroutine 51:
  content-storage-server/pkg/storage.(*QueuedWriteBatch).DoubleQueueCapacity()
      /home/dabajabaza/Nutstore/Work/Project/content-storage-server/pkg/storage/queue.go:393 +0x375
      TestCapacityDoublingDataRaceBetweenWorkerAndDoubler.func2()
      queue_race_test.go:204 +0xa4

Previous read at 0x00c0001e64d0 by goroutine 49:
  content-storage-server/pkg/storage.(*QueuedWriteBatch).worker()
      /home/dabajabaza/Nutstore/Work/Project/content-storage-server/pkg/storage/queue.go:152 +0x244
      startWorker.gowrap1()
      queue.go:123 +0x33
==================
```

### Test 2: TestQueueWriteDuringCapacityDoubling

```
==================
WARNING: DATA RACE #1
Write at queue.go:393 (DoubleQueueCapacity)
Read at queue.go:152 (worker)

WARNING: DATA RACE #2  
Write at queue.go:393 (DoubleQueueCapacity)
Read at queue.go:430 (QueueWrite)
==================

Completed - Total queued: 200, Processed: 60, Pending: 140, Errors: 0
```

---

## Root Cause Analysis

### The Problem

1. **Non-atomic snapshot:** `currentQueueLen := len(qwb.writeQueue)` (line 378) captures queue length at a single point in time
2. **Race during transfer:** While transferring tasks from old queue to new queue, other goroutines can still access `qwb.writeQueue`
3. **Non-atomic swap:** `qwb.writeQueue = newQueue` (line 393) is a simple assignment that is not protected by any synchronization

### Why This Is Dangerous

1. **Data Loss:** Tasks could be written to the old queue after it has been replaced
2. **Deadlock:** Worker could be waiting on a channel that no longer exists
3. **Panic:** Reading from a nil channel after swap
4. **Inconsistent State:** `maxQueueSize` and `writeQueue` could be out of sync

---

## Test Files Created

| File | Purpose |
|------|---------|
| `pkg/storage/queue_race_test.go` | Dedicated race condition tests |

### Test Functions

| Test Function | Description |
|---------------|-------------|
| `TestCapacityDoublingDataRaceBetweenWorkerAndDoubler` | Tests race between worker and capacity doubler |
| `TestQueueWriteDuringCapacityDoubling` | Tests race with concurrent writes to queue |
| `TestCapacityDoublingConcurrentWritesAndDoubles` | Stress test with multiple writers and doublers |
| `TestCapacityDoublingNoDataRaceMultipleRounds` | Multiple rounds of doubling under load |
| `TestCapacityDoublingStressWithImmediateWrites` | High stress with immediate writes |

---

## Running the Tests

### Confirm the Race (Current State)
```bash
# Run with race detector
go test -race -v -run "TestCapacityDoublingDataRaceBetweenWorkerAndDoubler" ./pkg/storage/...

# Expected: FAIL with "DATA RACE" warning
```

### Verify Fix (After Fix Applied)
```bash
# After implementing the fix, run again
go test -race -v -run "TestCapacityDoubling.*" ./pkg/storage/...

# Expected: PASS (no race detected)
```

---

## Impact Assessment

| Severity | Rating | Reason |
|----------|--------|--------|
| Likelihood | **Medium-High** | Race occurs during capacity doubling, which happens under load |
| Impact | **High** | Can cause data loss, deadlock, or panic |
| Overall | **HIGH** | Data race in production code |

---

## Potential Fix Approaches

### Approach 1: Mutex Protection (Simplest)
Protect all access to `writeQueue` with a mutex. Worker and QueueWrite would need to use mutex for read/write operations.

### Approach 2: Atomic Pointer Swap (Recommended)
Use `sync/atomic` to swap the channel pointer atomically:
```go
atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&qwb.writeQueue)), unsafe.Pointer(newQueue))
```

### Approach 3: Lock-Free Channel Handoff
Use a separate mechanism to signal the worker about the new queue instead of direct channel swap.

---

## Recommendations

1. **Do not deploy to production** until this race is fixed
2. **Monitor closely** if capacity doubling is triggered in production
3. **Consider adding circuit breaker** to prevent capacity doubling under extreme load
4. **Add observability** to detect when capacity doubling is happening (for debugging)

---

## Notes

- Issue #3 (AccessManager sync.Map) has been fixed separately
- Issue #1 (QueueDepth metric) and Issue #4 (Health status) remain unfixed
- All tests pass without `-race` flag, indicating the race is timing-dependent

---

## Test Output Summary

```
=== RUN   TestCapacityDoublingDataRaceBetweenWorkerAndDoubler
    queue_race_test.go:211: Completed 100 iterations with concurrent writes
==================
WARNING: DATA RACE
Write at 0x00c0001e64d0 by goroutine 51:
  DoubleQueueCapacity() at queue.go:393
Previous read at 0x00c0001e64d0 by goroutine 49:
  worker() at queue.go:152
==================
--- FAIL: TestCapacityDoublingDataRaceBetweenWorkerAndDoubler (0.16s)
FAIL
```

---

## Conclusion

**Issue #2 is confirmed: A data race exists in the capacity doubling logic.**

The race occurs because `DoubleQueueCapacity()` swaps the `writeQueue` channel without proper synchronization, while other goroutines (worker and QueueWrite) are concurrently accessing the same channel. This is a serious bug that can lead to data loss, deadlocks, or panics in production under load.

The tests written in `queue_race_test.go` successfully reproduce the race condition and can be used to verify any fix implementation.