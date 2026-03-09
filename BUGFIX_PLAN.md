# Bug Fix Implementation Plan

Based on CLIENT_TEST_2.md root cause analysis, here is the plan to fix all three bugs.

---

## Bug 1: ID Validation Inconsistency (HIGH)

### Problem
In performance mode, `ValidateID()` is never called during storage, so IDs with `..` are accepted. But retrieval always calls `ValidateID()`, causing rejection.

### Root Cause
- **Location:** `internal/handlers/validation.go`, lines 55-97
- Performance mode path (lines 58-90) only uses go-playground struct validation
- Conservative path (lines 93-126) explicitly calls `ValidateID()`

### Fix
Add explicit `ValidateID()` call to the performance mode path in `ValidateStorageRequest`:

```go
// internal/handlers/validation.go, in ValidateStorageRequest function
if v.config.PerformanceMode {
    if err := v.validator.Struct(req); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // ADD THIS: Explicit ID validation even in performance mode
    if err := v.ValidateID(req.ID); err != nil {
        return err
    }

    // ... rest of performance mode checks (content type, data size, expiration)
}
```

### Verification
- Run test_client.py --scenario id-edge
- ID `..hidden` should be rejected during storage (400)

---

## Bug 2: Access Limit = 1 Behavior Incorrect (HIGH)

### Problem
Setting `access_limit=1` causes immediate 410 on first access instead of allowing exactly one successful access.

### Root Cause
- **Location:** `pkg/storage/badger_CRUD.go`, lines 66-77
- Access count is incremented FIRST, then expiration is checked
- `IsExpired(1)` with limit=1: `1 >= 1` → TRUE → expired immediately

### Fix
Check access limit BEFORE incrementing in `getContentFromDB`:

```go
// pkg/storage/badger_CRUD.go, lines 66-77
if incrementAccess {
    // Check if already at limit BEFORE incrementing
    currentCount := s.accessManager.GetAccessCount(id)
    if content.AccessLimit > 0 && currentCount >= int64(content.AccessLimit) {
        return nil, ErrContentExpired
    }

    // Now safe to increment
    newAccessCount := s.accessManager.IncrementAccess(id)

    // Check time-based expiration (but not access-based since we pre-checked)
    if content.ExpiresAt != nil && time.Now().After(*content.ExpiresAt) {
        return nil, ErrContentExpired
    }
}
```

### Alternative Fix
Change the comparison in `IsExpired` to use `>` instead of `>=`:

```go
// pkg/models/content.go, line 51
// Change from:
if c.AccessLimit > 0 && accessCount >= int64(c.AccessLimit) {
// To:
if c.AccessLimit > 0 && accessCount > int64(c.AccessLimit) {
```

But this would require checking access limit BEFORE increment to avoid the race condition where two concurrent requests both pass the check.

**Recommended:** Check before increment approach (first option).

### Verification
- Run test_client.py --scenario access-boundary
- access_limit=1 should allow exactly one access, then 410

---

## Bug 3: Access Limit = 0 Allows Unlimited Access (MEDIUM)

### Problem
`access_limit=0` allows unlimited access instead of being rejected or expiring immediately.

### Root Cause
- **Location:** `pkg/models/content.go`, line 51
- Condition `c.AccessLimit > 0` means when AccessLimit=0, check is skipped entirely

### Fix
Add validation to reject `access_limit=0` during storage:

```go
// internal/handlers/validation.go
// Add new function:
func (v *RequestValidator) ValidateAccessLimit(limit int) error {
    if limit < 0 {
        return fmt.Errorf("access_limit cannot be negative")
    }
    if limit == 0 {
        return fmt.Errorf("access_limit must be greater than 0 (omit field for unlimited access)")
    }
    if limit > v.config.MaxAccessLimit {
        return fmt.Errorf("access_limit exceeds maximum of %d", v.config.MaxAccessLimit)
    }
    return nil
}

// Call it in ValidateStorageRequest (both paths):
if err := v.ValidateAccessLimit(req.AccessLimit); err != nil {
    return fmt.Errorf("invalid access limit: %w", err)
}
```

### Verification
- Run test_client.py --scenario access-boundary
- access_limit=0 should be rejected during storage (400)

---

## Implementation Order

1. **Bug 3 first** - Add validation for access_limit=0 (safest, prevents invalid data)
2. **Bug 2 second** - Fix access limit checking order (core logic fix)
3. **Bug 1 last** - Add ID validation to performance mode (consistency fix)

## Testing Strategy

After each fix:
1. Run guardian tests to ensure no regressions
2. Run specific test_client.py scenarios for the bug
3. Run full test_client.py suite

Final verification:
```bash
# Run all guardian tests
go test -v ./internal/handlers/ -run "TestAccessCount|TestExpired|TestNotFound|TestDelete|TestContentType|TestIsExpired"

# Run full client test
python3 test_client.py --url http://localhost:8082
```
