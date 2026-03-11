# Security Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix remaining medium-severity security issues identified in the security audit.

**Architecture:** Add configuration options and validation to make Swagger/management endpoints optionally require authentication, enforce minimum API key length, and add CSP header.

**Tech Stack:** Go, Echo framework, configuration validation

---

## Summary of Fixes

| Issue | Fix |
|-------|-----|
| Swagger publicly accessible | Add `ENABLE_SWAGGER` config (default: true for backwards compat) |
| Management interface public | Add `REQUIRE_AUTH_FOR_UI` config (default: false for backwards compat) |
| API key length not enforced | Change warning to error for keys < 32 chars |
| Missing CSP header | Add Content-Security-Policy to default security headers |

---

### Task 1: Add ENABLE_SWAGGER Configuration

**Files:**
- Modify: `pkg/config/config.go:115-117` (add field)
- Modify: `pkg/config/config.go:212-219` (add default)
- Modify: `pkg/middleware/auth.go:21-26` (check config)
- Modify: `pkg/config/config.go:416-480` (add validation)

**Step 1: Add ENABLE_SWAGGER config field**

In `pkg/config/config.go`, add to GROUP 9 (around line 115):

```go
EnableProfiler      bool              // Enable pprof endpoints
EnableSwagger       bool              // Enable Swagger documentation endpoints (default: true)
AllowedContentTypes []string          // Allowed request content types
```

**Step 2: Add default value (true for backwards compatibility)**

In `pkg/config/config.go` Load() function (around line 212):

```go
// Security settings
EnableProfiler:      envBool("ENABLE_PROFILER", false),
EnableSwagger:       envBool("ENABLE_SWAGGER", true),  // Enabled by default for backwards compatibility
AllowedContentTypes: envStringSlice("ALLOWED_CONTENT_TYPES", []string{"application/json", "text/plain"}),
```

**Step 3: Add validation warning**

In `pkg/config/config.go` Validate() function:

```go
if cfg.EnableSwagger && cfg.EnableAuth {
    warnings = append(warnings, "Swagger documentation is publicly accessible. Consider setting ENABLE_SWAGGER=false in production.")
}
```

**Step 4: Update auth middleware to check config**

In `pkg/middleware/auth.go`, modify the public paths check:

```go
// Skip authentication for health checks, heartbeat, profiler endpoints, and static files
path := c.Request().URL.Path
if path == "/health" || path == "/health/detailed" ||
	path == "/ping" || strings.HasPrefix(path, "/debug/") ||
	strings.HasPrefix(path, "/static/") || path == "/" ||
	(cfg.EnableSwagger && strings.HasPrefix(path, "/swagger/")) {
	return next(c)
}
```

**Step 5: Run tests to verify no regression**

Run: `go test ./pkg/middleware/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/config/config.go pkg/middleware/auth.go
git commit -m "feat(security): add ENABLE_SWAGGER config option

- Default true for backwards compatibility
- Warns when enabled with auth in production
- Allows disabling swagger in production"
```

---

### Task 2: Add REQUIRE_AUTH_FOR_UI Configuration

**Files:**
- Modify: `pkg/config/config.go:115-117` (add field)
- Modify: `pkg/config/config.go:212-219` (add default)
- Modify: `pkg/middleware/auth.go:21-26` (check config)

**Step 1: Add REQUIRE_AUTH_FOR_UI config field**

In `pkg/config/config.go`, add to GROUP 9:

```go
EnableProfiler      bool              // Enable pprof endpoints
EnableSwagger       bool              // Enable Swagger documentation endpoints
RequireAuthForUI    bool              // Require authentication for management UI (default: false)
AllowedContentTypes []string          // Allowed request content types
```

**Step 2: Add default value (false for backwards compatibility)**

In `pkg/config/config.go` Load() function:

```go
EnableProfiler:      envBool("ENABLE_PROFILER", false),
EnableSwagger:       envBool("ENABLE_SWAGGER", true),
RequireAuthForUI:    envBool("REQUIRE_AUTH_FOR_UI", false),  // Disabled by default for backwards compatibility
AllowedContentTypes: envStringSlice("ALLOWED_CONTENT_TYPES", []string{"application/json", "text/plain"}),
```

**Step 3: Add validation warning**

In `pkg/config/config.go` Validate() function:

```go
if !cfg.RequireAuthForUI && cfg.EnableAuth {
    warnings = append(warnings, "Management UI is publicly accessible. Consider setting REQUIRE_AUTH_FOR_UI=true in production.")
}
```

**Step 4: Update auth middleware**

In `pkg/middleware/auth.go`:

```go
// Skip authentication for health checks, heartbeat, profiler endpoints
path := c.Request().URL.Path

// Always public endpoints
if path == "/health" || path == "/health/detailed" || path == "/ping" {
	return next(c)
}

// Conditionally public endpoints
if strings.HasPrefix(path, "/debug/") {
	return next(c)
}
if !cfg.RequireAuthForUI && (strings.HasPrefix(path, "/static/") || path == "/") {
	return next(c)
}
if cfg.EnableSwagger && strings.HasPrefix(path, "/swagger/") {
	return next(c)
}
```

**Step 5: Run tests**

Run: `go test ./pkg/middleware/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/config/config.go pkg/middleware/auth.go
git commit -m "feat(security): add REQUIRE_AUTH_FOR_UI config option

- Default false for backwards compatibility
- Allows protecting management UI with authentication
- Warns when auth enabled but UI is public"
```

---

### Task 3: Enforce Minimum API Key Length

**Files:**
- Modify: `pkg/config/config.go:424-428` (change warning to error)

**Step 1: Change API key validation from warning to error**

In `pkg/config/config.go` Validate() function, change:

```go
if cfg.APIKey == "" && cfg.EnableAuth {
    errors = append(errors, "Authentication is enabled but API_KEY is empty. Set a secure API key.")
} else if cfg.APIKey != "" && len(cfg.APIKey) < 32 {
    warnings = append(warnings, fmt.Sprintf("API key is only %d characters. Recommend at least 32 characters for security.", len(cfg.APIKey)))
}
```

To:

```go
if cfg.APIKey == "" && cfg.EnableAuth {
    errors = append(errors, "Authentication is enabled but API_KEY is empty. Set a secure API key.")
} else if cfg.APIKey != "" && len(cfg.APIKey) < 32 {
    // Enforce minimum key length for security (can be bypassed with MIN_API_KEY_LENGTH=0 if needed)
    minKeyLength := envInt("MIN_API_KEY_LENGTH", 32)
    if minKeyLength > 0 && len(cfg.APIKey) < minKeyLength {
        errors = append(errors, fmt.Sprintf("API key must be at least %d characters (got %d). Set MIN_API_KEY_LENGTH=0 to bypass.", minKeyLength, len(cfg.APIKey)))
    }
}
```

Wait - we can't use env() in Validate() since it's called after Load(). Let me revise:

**Step 1: Add MinAPIKeyLength config field**

In `pkg/config/config.go`, add to GROUP 5 (auth settings):

```go
APIKey           string   // API key for authentication
MinAPIKeyLength  int      // Minimum API key length (0 to disable enforcement)
EnableAuth       bool     // Enable API key authentication
```

**Step 2: Add default value**

In Load() function:

```go
// Security settings
APIKey:         env("API_KEY", ""),
MinAPIKeyLength: envInt("MIN_API_KEY_LENGTH", 32),  // Minimum 32 chars, set to 0 to bypass
EnableAuth:     envBool("ENABLE_AUTH", true),
```

**Step 3: Update validation**

```go
if cfg.APIKey == "" && cfg.EnableAuth {
    errors = append(errors, "Authentication is enabled but API_KEY is empty. Set a secure API key.")
} else if cfg.APIKey != "" && cfg.MinAPIKeyLength > 0 && len(cfg.APIKey) < cfg.MinAPIKeyLength {
    errors = append(errors, fmt.Sprintf("API key must be at least %d characters (got %d). Set MIN_API_KEY_LENGTH=0 to bypass.", cfg.MinAPIKeyLength, len(cfg.APIKey)))
}
```

**Step 4: Run tests**

Run: `go test ./pkg/config/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat(security): enforce minimum API key length (32 chars)

- Default minimum 32 characters
- Configurable via MIN_API_KEY_LENGTH
- Set to 0 to bypass enforcement
- Returns error (not warning) for short keys"
```

---

### Task 4: Add Content-Security-Policy Header

**Files:**
- Modify: `pkg/config/config.go:215-219` (add header)

**Step 1: Add CSP header to defaults**

In `pkg/config/config.go` Load() function:

```go
SecurityHeaders: envStringMap("SECURITY_HEADERS", map[string]string{
    "X-Content-Type-Options":  "nosniff",
    "X-Frame-Options":         "DENY",
    "X-XSS-Protection":        "1; mode=block",
    "Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
}),
```

Note: `unsafe-inline` for scripts/styles is needed for the management UI to function.

**Step 2: Verify headers are applied**

Run: `go test ./pkg/middleware/... -v -run TestSecure`
Expected: PASS (or verify manually with curl)

**Step 3: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat(security): add Content-Security-Policy header

- Adds CSP to default security headers
- Allows 'self' plus inline scripts/styles for UI
- Can be customized via SECURITY_HEADERS env var"
```

---

### Task 5: Update Security Audit Report

**Files:**
- Modify: `notes/SECURITY_AUDIT_REPORT.md`

**Step 1: Update report with new fixes**

Mark the following issues as FIXED:
- Swagger publicly accessible -> Now configurable with ENABLE_SWAGGER
- Management interface public -> Now configurable with REQUIRE_AUTH_FOR_UI
- API key length not enforced -> Now enforced with MIN_API_KEY_LENGTH
- Missing CSP header -> Added to defaults

**Step 2: Commit**

```bash
git add notes/SECURITY_AUDIT_REPORT.md
git commit -m "docs: update security audit with new fixes"
```

---

### Task 6: Final Verification

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

**Step 2: Run the server and verify config**

Run: `go build -o server ./cmd/server && ./server &`
Check: Verify startup messages show new config options

**Step 3: Test new config options**

```bash
# Test ENABLE_SWAGGER=false
ENABLE_SWAGGER=false ./server &
curl http://localhost:8081/swagger/index.html  # Should return 401 if auth enabled

# Test REQUIRE_AUTH_FOR_UI=true
REQUIRE_AUTH_FOR_UI=true ./server &
curl http://localhost:8081/  # Should return 401 if auth enabled

# Test MIN_API_KEY_LENGTH
API_KEY=short ENABLE_AUTH=true ./server  # Should fail with error
```

**Step 4: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: address any remaining test failures"
```

---

## Production Configuration After Fixes

```bash
# Recommended production configuration
ENABLE_AUTH=true
API_KEY=<secure-random-key-min-32-chars>
MIN_API_KEY_LENGTH=32

# TLS
ENABLE_TLS=true
TLS_HOSTS=api.yourdomain.com
ENABLE_HTTPS_ONLY=true

# Security options (NEW)
ENABLE_SWAGGER=false          # Disable swagger in production
REQUIRE_AUTH_FOR_UI=true      # Protect management UI

# CORS
ALLOWED_ORIGINS=https://yourdomain.com
```

---

## Files Changed Summary

| File | Changes |
|------|---------|
| `pkg/config/config.go` | Add EnableSwagger, RequireAuthForUI, MinAPIKeyLength fields; Add CSP header; Update validation |
| `pkg/middleware/auth.go` | Check new config flags for swagger/UI paths |
| `notes/SECURITY_AUDIT_REPORT.md` | Document fixes |

---

**Plan complete and saved to `docs/plans/2026-03-11-security-fixes.md`. Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
