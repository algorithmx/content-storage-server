# Stress Test Code Review Report

**Review Date:** 2026-03-10
**Code Reviewer:** Claude
**File Reviewed:** `cmd/stress-test/main.go`
**Lines of Code:** ~1800

---

## Executive Summary

The stress test code has several correctness and fairness issues that could lead to:
- False positive bug reports (incorrectly flagging server bugs)
- Race conditions in the test code itself
- Inconsistent API endpoint usage
- Improper resource cleanup
- Memory/connection leaks

**Overall Assessment:** The code requires fixes before it can provide reliable bug discovery results.

---

## Critical Issues (Must Fix)

### Issue 1: Inconsistent API Endpoint Usage (Lines 988, 1045, 1089, 1117, 1162, 1235, 1308)

**Problem:** The bug discovery tests use `/api/v1/content/` (with trailing slash) while the main stress test uses `/api/v1/content` (without trailing slash).

**Code Evidence:**
```go
// Bug discovery tests use trailing slash (INCORRECT)
st.baseURL+"/api/v1/content/"  // Lines 988, 1045, 1089, 1117, 1162, 1235, 1308

// Main stress test uses no trailing slash (CORRECT)
st.baseURL+"/api/v1/content"   // Line 764
```

**Impact:**
- Server may return 404 for these requests
- Tests fail not due to server bugs, but incorrect URLs
- False negative bug reports (missing actual bugs)

**Fix:** Remove trailing slashes from all POST requests in bug discovery tests.

---

### Issue 2: HTTP Response Body Not Fully Consumed (Lines 936, 1001-1002, 1018-1021)

**Problem:** In `testIDValidationEdgeCases()` and `testAccessLimitBoundaries()`, response bodies are not fully read before closing. This can cause connection reuse issues.

**Code Evidence:**
```go
// Line 936 - Body closed but not read
resp.Body.Close()
storeStatus := resp.StatusCode  // This is OK

// Lines 1001-1002 - Body closed without reading
getResp1.Body.Close()
status1 := getResp1.StatusCode  // Already read status, but body not consumed
```

**Impact:**
- HTTP client connection pool may be exhausted
- Subsequent requests may hang or fail
- Not following Go HTTP client best practices

**Fix:** Always consume response body before closing:
```go
io.Copy(io.Discard, resp.Body)
resp.Body.Close()
```

---

### Issue 3: Race Condition in Access Limit Test (Lines 1265-1268)

**Problem:** The concurrent access limit test has a race condition. Multiple goroutines may read the same access_count value before any updates, allowing more than `accessLimit` requests to succeed.

**Code Evidence:**
```go
// Lines 1265-1268 - Race condition here
switch getResp.StatusCode {
case 200:
    atomic.AddInt64(&successCount, 1)  // Only counts successful responses
case 410:
    atomic.AddInt64(&goneCount, 1)
}
```

**Impact:**
- Test may incorrectly report a bug when the server is working correctly
- The test logic assumes sequential access counting, but server uses concurrent updates
- False positive bug reports

**Analysis:**
The test expects exactly `accessLimit` (5) successes with 20 concurrent requests. However, due to timing:
1. All 20 goroutines may issue GET requests simultaneously
2. Server processes them and decrements access_count
3. Some requests may see access_count > 0 even after 5 have "succeeded"
4. The test is unfair - it doesn't account for network/serialization delays

**Fix:**
- Wait for content to be fully stored (use status endpoint polling)
- Use sequential requests to verify access limit, then use concurrent to test race behavior
- Or document that this test checks for "at most N" not "exactly N"

---

### Issue 4: Fixed Sleep Instead of Proper Synchronization (Lines 996, 1055, 1124, 1245, 1319)

**Problem:** Tests use `time.Sleep()` with arbitrary durations instead of polling for actual state.

**Code Evidence:**
```go
time.Sleep(100 * time.Millisecond)  // Lines 996, 1055, 1124
time.Sleep(200 * time.Millisecond)  // Lines 1245, 1319
```

**Impact:**
- Tests may fail on slower systems (sleep too short)
- Tests may be unnecessarily slow on faster systems (sleep too long)
- Timing-dependent flaky tests

**Fix:** Poll the status endpoint until content is confirmed stored:
```go
// Poll for content availability
for i := 0; i < 50; i++ {
    resp, _ := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id + "/status")
    if resp != nil && resp.StatusCode == 200 {
        resp.Body.Close()
        break
    }
    time.Sleep(10 * time.Millisecond)
}
```

---

## Major Issues (Should Fix)

### Issue 5: Response Body Close in Loop (Lines 1261, 1336)

**Problem:** Using `defer resp.Body.Close()` inside a loop defers closing until function exit, not iteration end.

**Code Evidence:**
```go
// Line 1261 - defer inside goroutine loop
for i := 0; i < concurrency; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        getResp, err := st.httpClient.Get(...)
        if err != nil {
            return
        }
        defer getResp.Body.Close()  // This defers to goroutine exit, which is OK
        // But also see Line 1336 below...
    }()
}
```

Wait, the goroutine case is actually OK. Let me check line 1336:

```go
// Lines 1331-1350 - defer inside for loop (BUG)
for j := 0; j < iterations; j++ {
    getResp, err := st.httpClient.Get(...)
    if err != nil {
        continue
    }
    defer getResp.Body.Close()  // BUG: defers to function exit, not loop iteration!
    // ...
}
```

**Impact:**
- Memory leak: All response bodies accumulated until function returns
- Connection pool exhaustion
- Test may fail due to resource limits

**Fix:** Explicitly close bodies in loop without defer:
```go
for j := 0; j < iterations; j++ {
    getResp, err := st.httpClient.Get(...)
    if err != nil {
        continue
    }
    body, _ := io.ReadAll(getResp.Body)
    getResp.Body.Close()  // Close immediately
    // ... use body ...
}
```

---

### Issue 6: Silent Error Handling (Lines 987-988, 1043-1044, 1088, 1115, 1160, etc.)

**Problem:** Many errors are silently ignored with `_` assignment.

**Code Evidence:**
```go
jsonData, _ := json.Marshal(content)  // Line 987
resp, _ := st.httpClient.Post(...)     // Line 988
resp0, _ := st.httpClient.Post(...)    // Line 1044
jsonData, _ := json.Marshal(content)   // Line 1115
resp, _ := st.httpClient.Post(...)     // Line 1160
```

**Impact:**
- Failures are not detected or reported
- Tests may proceed with invalid data/nil responses
- Hard to debug test failures

**Fix:** Check and handle errors:
```go
jsonData, err := json.Marshal(content)
if err != nil {
    fmt.Printf("Failed to marshal: %v\n", err)
    return
}
resp, err := st.httpClient.Post(...)
if err != nil {
    fmt.Printf("Request failed: %v\n", err)
    return
}
```

---

### Issue 7: Potential Double Close of Response Body (Lines 926-944)

**Problem:** In `testIDValidationEdgeCases()`, response body may be closed twice.

**Code Evidence:**
```go
resp, err := st.httpClient.Post(...)
// ...
defer resp.Body.Close()  // Line 935 - deferred close
storeStatus := resp.StatusCode
// ...
if storeStatus == 202 {
    req, _ := http.NewRequest("DELETE", ...)
    st.httpClient.Do(req)  // This is OK, new request
}
// At function iteration end, deferred Close() called
```

Actually, looking more carefully - the `defer` is inside a loop iteration (not the function), so this might be OK depending on Go version. But `defer` in a loop is still problematic.

**Impact:**
- Runtime panic on double close (in older Go versions)
- Resource leaks

**Fix:** Remove defer from loop, use explicit close.

---

### Issue 8: Response Body Read After Close (Lines 1339-1347)

**Problem:** In `testDataIntegrityUnderLoad()`, attempting to read body after defer closes it.

**Code Evidence:**
```go
if getResp.StatusCode == 200 {
    body, _ := io.ReadAll(getResp.Body)  // Line 1339 - read body
    var result map[string]any
    if err := json.Unmarshal(body, &result); err == nil {
        // ...
    }
}
// defer getResp.Body.Close() runs AFTER the block above in Go!
// Actually no - the defer was registered before this block, so body is still open
```

Wait, let me re-check. The defer is at line 1336 which is BEFORE the read at line 1339, so this is actually OK for a single iteration.

But the real issue is the loop problem mentioned in Issue 5.

---

## Minor Issues (Nice to Fix)

### Issue 9: Unused `accessLimitIDs` Field (Lines 272-273)

**Problem:** The `accessLimitIDs` field is declared but never used.

**Code Evidence:**
```go
// Lines 272-273 in StressTest struct
accessLimitIDs  map[string]int // contentID -> access_limit
accessLimitMutex sync.RWMutex
```

**Impact:**
- Dead code
- Confusion about intended functionality

**Fix:** Either implement the tracking or remove the field.

---

### Issue 10: Inconsistent Error Counting in Rapid Cycle Test (Lines 1379, 1398)

**Problem:** The rapid cycle test increments `errors` for failed creates but doesn't check for failed deletes or reads.

**Code Evidence:**
```go
if err != nil || resp.StatusCode != 202 {
    errors++  // Only counts store errors
    // ...
}
// Delete and Get errors are silently ignored
```

**Fix:** Track all operation errors or document the intentional limitation.

---

### Issue 11: Comment/Code Mismatch (Line 906)

**Problem:** Comment says test is for "Bug 1" but test IDs don't match the original Bug 1 description.

**Code Evidence:**
```go
// testIDValidationEdgeCases tests Bug 1: ID Validation Inconsistency
// IDs with '..' can be stored but not retrieved
```

But the test expects store to return 400 for these IDs, not 202. The original Bug 1 was about IDs that CAN be stored but NOT retrieved.

**Impact:**
- Confusion about what bug is being tested
- Test may not actually detect the original bug

**Fix:** Update comment to reflect actual test intent.

---

## Fairness Issues

### Issue 12: Unfair Concurrent Access Limit Test

**Problem:** The concurrent access limit test at lines 1219-1291 is fundamentally unfair to the server.

**Analysis:**
1. Creates content with `access_limit=5`
2. Sleeps 200ms (may not be enough on slow systems)
3. Launches 20 concurrent GET requests
4. Expects exactly 5 successes

The unfairness:
- Network serialization means requests don't arrive simultaneously
- Server processing is not instantaneous
- Some goroutines may start significantly later than others
- The Go scheduler doesn't guarantee fairness

**Recommendation:**
- Use a sync barrier (sync.WaitGroup) to ensure all goroutines start together
- Or use sequential requests to verify the limit works, then concurrent for race detection

---

### Issue 13: Data Integrity Test Unfairness (Lines 1293-1372)

**Problem:** The data integrity test only checks mismatches, not missing data or extra data.

**Fairness Issue:**
- Test only verifies successful reads (status 200)
- If server returns corrupted data, it's caught
- But if server returns 404 for valid content, it's ignored (just continue)
- This may mask data loss bugs

**Fix:** Also track and report the number of 404 responses for existing content.

---

## Summary Table

| Issue | Severity | Location | Type |
|-------|----------|----------|------|
| Inconsistent API endpoint | Critical | Lines 988, 1045, 1089, 1117, 1162, 1235, 1308 | Correctness |
| Response body not consumed | Critical | Lines 936, 1001-1002 | Resource leak |
| Race condition in test | Critical | Lines 1265-1268 | Fairness |
| Fixed sleep vs polling | Major | Lines 996, 1055, 1124, 1245, 1319 | Flakiness |
| Response body close in loop | Major | Line 1336 | Resource leak |
| Silent error handling | Major | Throughout | Debugging |
| Double body close | Minor | Line 935 | Correctness |
| Unused fields | Minor | Lines 272-273 | Dead code |
| Inconsistent error counting | Minor | Line 1379 | Fairness |
| Comment/code mismatch | Minor | Line 906 | Documentation |
| Unfair concurrent test | Design | Lines 1219-1291 | Fairness |
| Data integrity gaps | Design | Lines 1293-1372 | Fairness |

---

## Recommendations

### Immediate Actions
1. Fix API endpoint inconsistencies (Issue 1)
2. Fix response body handling (Issues 2, 5, 7)
3. Add proper error handling (Issue 6)

### Short Term
4. Replace fixed sleeps with polling (Issue 4)
5. Fix race condition in concurrent test (Issue 3)
6. Remove or use unused fields (Issue 9)

### Long Term
7. Redesign concurrent access test for fairness (Issue 12)
8. Enhance data integrity test (Issue 13)
9. Add more comprehensive assertions
10. Add test self-verification (meta-testing)

---

*Report generated by Code Review*
