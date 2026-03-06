# Content Storage Server - Issue Report

**Test Date:** 2026-03-06  
**Test Client:** Python test_client.py  
**Server Version:** Content Storage Server (Go/BadgerDB)  
**Test Environment:** localhost:8082

---

## Executive Summary

The comprehensive test client executed **15 realistic client scenarios** against the content storage server. While core functionality (CRUD, async queue, pagination, health endpoints) works correctly, **6 issues** were identified that violate the server's documented API contract.

**Test Results:**
- Passed: 45 tests
- Failed: 0 tests  
- Issues Found: 6
- Warnings: 1

---

## Issues Detail

### Issue 1: Access Count Incremented Twice on Retrieval

**Severity:** Medium
**Category:** Data Integrity

**Description:** When retrieving content, the access count is incremented twice instead of once.

**Test Evidence:**
```
[Access Count] Access count should be 1 after first retrieval
  Details: { "expected": 1, "actual": 2 }
```

**Root Cause Analysis:**

The bug is in `internal/handlers/content.go` in the `GetContentStatus` function (lines 583-601):

```go
// Check if content exists in storage (persisted)
_, err := h.storage.Get(id)  // This increments access count!
```

The `GetContentStatus` handler calls `h.storage.Get(id)` which internally calls `getContentFromDB(id, true)` with `incrementAccess=true`. This increments the access count every time the status endpoint is polled.

Additionally, when the client retrieves content via `GetContent`, it also calls `h.storage.Get(id)`, causing a second increment.

**Code Flow:**
1. Client stores content → queued
2. Client polls status via `GetContentStatus` → increments count to 1
3. Client retrieves content via `GetContent` → increments count to 2
4. First retrieval shows access_count=2 instead of 1

**Fix:** The `GetContentStatus` handler should use `GetReadOnly` or pass `incrementAccess=false` to avoid incrementing the count during status checks.

**Impact:**
- Access limits will be reached twice as fast as intended
- Analytics on content access are incorrect
- Rate limiting based on access count is broken

---

### Issue 2: Access Limit Enforcement Returns HTTP 500

**Severity:** High
**Category:** API Contract Violation

**Description:** When content reaches its access limit, the server returns HTTP 500 Internal Server Error instead of HTTP 410 Gone.

**Test Evidence:**
```
[Access Limit] Expected 200 on access 3/3, got 500
[Access Limit Enforcement] Expected 410 after access limit reached, got 500
  Details: { "expected": 410, "actual": 500, "access_limit": 3 }
```

**Root Cause Analysis:**

Two issues in `internal/handlers/content.go`:

1. **Missing error handling for `ErrContentExpired`**: The `GetContent` handler (lines 193-217) only checks for `ErrContentNotFound` but not `ErrContentExpired`:

```go
content, err := h.storage.Get(id)
if err != nil {
    if err == storage.ErrContentNotFound {
        return c.JSON(http.StatusNotFound, ...)
    }
    // Falls through to 500 error for ErrContentExpired!
    h.logError("Failed to retrieve content", ...)
    return c.JSON(http.StatusInternalServerError, ...)  // BUG: Returns 500
}
```

2. **Logic error in expiration check**: In `pkg/models/content.go` line 51:

```go
func (c *Content) IsExpired(accessCount int64) bool {
    // Check access count-based expiration
    // Content expires when access count exceeds the limit (not when it equals)
    if c.AccessLimit > 0 && accessCount > int64(c.AccessLimit) {
        return true
    }
}
```

The condition is `accessCount > AccessLimit`, which means with AccessLimit=3:
- After 3rd access: count=3, 3>3? No → NOT expired (BUG - should be expired)
- After 4th access: count=4, 4>3? Yes → Expired

This allows one extra access beyond the limit. Combined with Issue 1 (double increment), this causes unexpected behavior.

**Fix:**
1. Add check for `ErrContentExpired` in `GetContent` handler and return 410
2. Change condition to `accessCount >= AccessLimit` or handle equality case separately

**Expected Behavior (per OpenAPI spec):**
```
/api/v1/content/{id}:
  get:
    responses:
      '410':
        description: Content access limit reached
```

**Impact:**
- Client applications cannot properly handle expired content
- Breaks retry logic and cache invalidation in clients
- Indicates unhandled error in access limit checking logic

---

### Issue 3: Time-Based Expiration Returns HTTP 500

**Severity:** High  
**Category:** API Contract Violation

**Description:** When content expires due to time-based expiration, the server returns HTTP 500 instead of HTTP 404.

**Test Evidence:**
```
[Time Expiration] Expected 404 for expired content, got 500
  Details: { "expected": 404, "actual": 500 }
```

**Root Cause Analysis:**

Same root cause as Issue 2. In `internal/handlers/content.go` `GetContent` handler (lines 193-217), when content expires due to time-based expiration (`ExpiresAt`), the storage layer returns `ErrContentExpired` from `pkg/storage/badger_CRUD.go` line 73:

```go
// Check expiration after incrementing (this allows exactly AccessLimit accesses)
if content.IsExpired(newAccessCount) {
    return nil, ErrContentExpired
}
```

However, the handler doesn't check for this error type:

```go
content, err := h.storage.Get(id)
if err != nil {
    if err == storage.ErrContentNotFound {
        return c.JSON(http.StatusNotFound, ...)
    }
    // Falls through to 500 error for time-expired content!
    return c.JSON(http.StatusInternalServerError, ...)  // BUG: Returns 500
}
```

**Fix:** Add check for `ErrContentExpired` and return HTTP 404 (as per OpenAPI spec which groups "not found" and "expired" into 404).

**Expected Behavior (per OpenAPI spec):**
```
/api/v1/content/{id}:
  get:
    responses:
      '404':
        description: Content not found or expired
```

**Impact:**
- Clients cannot distinguish between "content never existed" and "content expired"
- Breaks idempotency guarantees
- Indicates error in expiration checking logic

---

### Issue 4: Delete Non-Existent Content Returns HTTP 200

**Severity:** Low  
**Category:** API Contract Violation

**Description:** Attempting to delete content that doesn't exist returns HTTP 200 instead of HTTP 404.

**Test Evidence:**
```
[Delete Non-existent] Expected 404 for deleting non-existent content, got 200
```

**Root Cause Analysis:**

In `pkg/storage/badger_CRUD.go` lines 133-138:

```go
err = s.db.Update(func(txn *badger.Txn) error {
    deleteErr := txn.Delete([]byte(id))
    if deleteErr == badger.ErrKeyNotFound {
        return ErrContentNotFound
    }
    return deleteErr
})
```

The code checks for `badger.ErrKeyNotFound`, but according to BadgerDB documentation and behavior, `txn.Delete()` does NOT return an error when the key doesn't exist - it returns success (nil). The check is essentially dead code.

Additionally, even if `ErrContentNotFound` were returned, the handler in `internal/handlers/content.go` uses direct error comparison (`err == storage.ErrContentNotFound`) which fails for wrapped errors. It should use `errors.Is()`.

**Fix:**
1. Check if content exists before deleting by attempting a read first
2. Use `errors.Is()` for error comparison in the handler

**Root Cause Analysis:**

In `pkg/storage/badger_CRUD.go` lines 133-138:

```go
err = s.db.Update(func(txn *badger.Txn) error {
    deleteErr := txn.Delete([]byte(id))
    if deleteErr == badger.ErrKeyNotFound {
        return ErrContentNotFound
    }
    return deleteErr
})
```

The code checks for `badger.ErrKeyNotFound`, but according to BadgerDB documentation and behavior, `txn.Delete()` does NOT return an error when the key doesn't exist - it returns success silently. Therefore, `ErrContentNotFound` is never returned.

**Fix:** Check if content exists before attempting deletion:

```go
err = s.db.Update(func(txn *badger.Txn) error {
    // First check if key exists
    _, getErr := txn.Get([]byte(id))
    if getErr == badger.ErrKeyNotFound {
        return ErrContentNotFound
    }
    // Now delete
    return txn.Delete([]byte(id))
})
```

**Root Cause Analysis:**

The bug is in `pkg/storage/badger_CRUD.go` lines 133-138:

```go
err = s.db.Update(func(txn *badger.Txn) error {
    deleteErr := txn.Delete([]byte(id))
    if deleteErr == badger.ErrKeyNotFound {
        return ErrContentNotFound
    }
    return deleteErr
})
```

**The Problem:** BadgerDB's `txn.Delete()` does NOT return an error when the key doesn't exist. According to BadgerDB documentation and behavior, Delete is idempotent - deleting a non-existent key succeeds silently.

The code checks for `badger.ErrKeyNotFound`, but this error is never returned by Delete operations. The check is effectively dead code.

**Fix:** Check if content exists before attempting deletion:

```go
// First check if content exists
_, err := s.GetReadOnly(id)
if err != nil {
    return ErrContentNotFound
}
// Then delete
return s.db.Update(func(txn *badger.Txn) error {
    return txn.Delete([]byte(id))
})
```

**Expected Behavior (per OpenAPI spec):**
```
/api/v1/content/{id}:
  delete:
    responses:
      '404':
        description: Content not found
```

**Impact:**
- Violates REST best practices
- Makes it impossible to distinguish between "deleted successfully" and "didn't exist"
- May cause silent failures in client applications

---

### Issue 5: Access Count Atomicity Under Concurrent Load

**Severity:** Medium  
**Category:** Concurrency

**Description:** Under concurrent access, the final access count doesn't match the number of successful accesses.

**Test Evidence:**
```
[Access Count Atomicity] Final count doesn't match number of accesses
  Details: { "final_count": 22, "successful_reads": 20, "expected_count": 21 }
```

**Root Cause Analysis:**

Two contributing factors:

1. **Double increment issue (Issue 1)**: The test polls status before retrieving content, causing extra increments via `GetContentStatus`.

2. **Test timing issue**: The test captures successful reads (20) but by the time it fetches the final count, additional accesses may have occurred due to:
   - Concurrent goroutines still completing
   - Status check endpoint also increments counts

The atomic operations in `pkg/models/content.go` are actually correct:

```go
func (at *AccessTracker) IncrementAccessCount() int64 {
    return atomic.AddInt64(&at.AccessCount, 1)
}
```

However, the test observed `final_count=22` when `expected=21`. This discrepancy is explained by:
- 20 successful concurrent reads
- Plus 1 final read to get the count = 21 expected
- But actual was 22 due to `GetContentStatus` polling during the test

**Fix:** Fix Issue 1 first, then re-test. The atomic operations themselves appear correct.

**Note:** The real concurrency issue would manifest as lost updates if atomic operations weren't used. The current implementation using `atomic.AddInt64` should be thread-safe.

**Impact:**
- Access limits may trigger prematurely or not at all under load
- Analytics data is unreliable
- Cannot trust access counts for billing/usage tracking

---

### Issue 6: Content Type Validation Not Enforced

**Severity:** Low  
**Category:** Input Validation

**Description:** Server accepts any content type, not enforcing the `ALLOWED_CONTENT_TYPES` configuration.

**Test Evidence:**
```
[WARN] Server accepts any content type (415 not enforced)
```

**Root Cause Analysis:**

In `internal/handlers/validation.go` lines 56-77, when `PerformanceMode` is enabled (default), the `ValidateStorageRequest` function takes a fast path that skips content type validation:

```go
func (v *RequestValidator) ValidateStorageRequest(req *models.StorageRequest) error {
    if v.config.PerformanceMode {
        // Fast path: Use go-playground/validator for basic struct validation
        if err := v.validator.Struct(req); err != nil {
            return fmt.Errorf("validation failed: %w", err)
        }

        // Only perform expensive validations if necessary
        // Quick data size check without full content validation
        if len(req.Data) > int(v.config.MaxContentSize) {
            return fmt.Errorf("data size (%d bytes) exceeds maximum allowed size (%d bytes)",
                len(req.Data), v.config.MaxContentSize)
        }

        // Quick expiration time check
        if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now()) {
            return fmt.Errorf("expiration time cannot be in the past")
        }

        return nil  // BUG: Never checks AllowedContentTypes!
    }

    // Conservative path: Full validation (for non-performance mode)
    // ... includes content type validation but is skipped in performance mode
}
```

The `AllowedContentTypes` configuration is only checked in the "conservative path" (lines 79-113), which is never executed when PerformanceMode is enabled.

**Fix:** Add content type validation to the performance mode path:

```go
if v.config.PerformanceMode {
    // ... existing checks ...

    // Check allowed content types
    if len(v.config.AllowedContentTypes) > 0 {
        allowed := false
        for _, allowedType := range v.config.AllowedContentTypes {
            if req.Type == allowedType {
                allowed = true
                break
            }
        }
        if !allowed {
            return fmt.Errorf("content type '%s' not in allowed types", req.Type)
        }
    }

    return nil
}
```

**Expected Behavior (per OpenAPI spec):**
```
POST /api/v1/content:
  responses:
    '415':
      description: Content type not in ALLOWED_CONTENT_TYPES
```

**Impact:**
- Configuration parameter has no effect
- May lead to unexpected content types being stored
- Security concern if content type impacts processing

---

## Working Functionality

The following server features work correctly:

| Feature | Status |
|---------|--------|
| Basic CRUD operations | ✅ Pass |
| Async write queue (202 response) | ✅ Pass |
| Content status (queued/stored) | ✅ Pass |
| Sequential write ordering | ✅ Pass |
| Pagination & filtering | ✅ Pass |
| Health endpoints | ✅ Pass |
| Metrics endpoint | ✅ Pass |
| Management operations (backup, GC) | ✅ Pass |
| Content overwrite (last-write-wins) | ✅ Pass |
| Large content (100KB) handling | ✅ Pass |
| Concurrent write operations | ✅ Pass |

---

## Recommendations

### High Priority:

1. **Fix Issue 2 & 3 (500 errors):**
   - File: `internal/handlers/content.go`
   - In `GetContent` handler (lines 193-217), add check for `ErrContentExpired`:
   ```go
   if err != nil {
       if err == storage.ErrContentNotFound {
           return c.JSON(http.StatusNotFound, ...)
       }
       if err == storage.ErrContentExpired {
           // Check if expired due to access limit or time
           // Return 410 for access limit, 404 for time expiration
           return c.JSON(http.StatusGone, ...) // or 404
       }
       // ... 500 error for actual internal errors
   }
   ```

2. **Fix Issue 4 (Delete returns 200):**
   - File: `pkg/storage/badger_CRUD.go`
   - In `Delete` function (lines 114-158), add existence check before delete:
   ```go
   func (s *BadgerStorage) Delete(id string) error {
       // First verify content exists
       _, err := s.GetReadOnly(id)
       if err != nil {
           return err // Returns ErrContentNotFound if not found
       }
       // ... proceed with deletion
   }
   ```

### Medium Priority:

3. **Fix Issue 1 (Double access count increment):**
   - File: `internal/handlers/content.go`
   - In `GetContentStatus` handler (line 583), use read-only method:
   ```go
   // Replace: _, err := h.storage.Get(id)
   // With:    _, err := h.storage.GetReadOnly(id)
   ```

4. **Fix Issue 5 (Access limit logic):**
   - File: `pkg/models/content.go`
   - Change line 51 from `>` to `>=`:
   ```go
   if c.AccessLimit > 0 && accessCount >= int64(c.AccessLimit) {
       return true
   }
   ```

### Low Priority:

5. **Fix Issue 6 (Content type validation):**
   - File: `internal/handlers/validation.go`
   - Add `AllowedContentTypes` check in performance mode path (around line 76)

---

## Test Command

To reproduce issues:
```bash
python3 test_client.py --url http://localhost:8082
```

To test specific scenarios:
```bash
python3 test_client.py --url http://localhost:8082 --scenario access
python3 test_client.py --url http://localhost:8082 --scenario expiration
```