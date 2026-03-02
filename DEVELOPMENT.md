# Development Guide

This guide provides comprehensive information for contributors to the Content Storage Server project.

## Table of Contents

1. [Development Environment Setup](#development-environment-setup)
2. [Project Structure](#project-structure)
3. [Architecture Overview](#architecture-overview)
4. [Technology Stack](#technology-stack)
5. [Configuration Reference](#configuration-reference)
6. [API Documentation](#api-documentation)
7. [Coding Standards](#coding-standards)
8. [Testing Guidelines](#testing-guidelines)
9. [Development Workflow](#development-workflow)
10. [Pull Request Process](#pull-request-process)
11. [Code Review Checklist](#code-review-checklist)
12. [Release Process](#release-process)

---

## Development Environment Setup

### Prerequisites

- Go 1.24 or later
- Docker (for containerized testing)
- git
- Make (optional, for build automation)

### Initial Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/content-storage-server.git
cd content-storage-server

# Add upstream remote
git remote add upstream https://github.com/ORIGINAL_OWNER/content-storage-server.git

# Install dependencies
go mod download

# Run tests to verify setup
go test ./...
```

### Recommended Tools

- **GoLand** or **VSCode** with Go extension
- **dlv** for debugging (`go install github.com/go-delve/delve/cmd/dlv@latest`)
- **golangci-lint** for linting
- **swag** for Swagger doc generation (`go install github.com/swaggo/swag/cmd/swag@latest`)

---

## Project Structure

### Directory Overview

```
content-storage-server/
├── cmd/                        # Main applications
│   ├── server/                 # HTTP server application
│   │   └── main.go            # Entry point with Echo v4 setup
│   └── stress-test/            # Load testing tool
├── internal/                   # Private application code
│   └── handlers/               # HTTP request handlers
│       ├── content.go          # Content CRUD operations
│       ├── health.go           # Health check endpoints
│       ├── shutdown.go         # Graceful/emergency shutdown
│       ├── validation.go       # Request validation
│       ├── errors.go           # Error handling utilities
│       └── swagger_models.go   # Swagger API models
├── pkg/                        # Public library code
│   ├── config/                 # Configuration management
│   ├── logger/                 # Zap-based structured logging
│   ├── middleware/             # HTTP middleware stack
│   ├── models/                 # Content, AccessTracker models
│   ├── storage/                # Storage layer (see expanded below)
│   └── tls/                    # AutoTLS for HTTPS
├── static/                     # Web management UI
│   ├── index.html
│   ├── css/custom.css
│   └── js/management.js
├── docs/                       # Generated Swagger docs
├── .env.example                # Configuration template
├── Dockerfile                  # Multi-stage container build
├── docker-compose.yml          # Development environment
├── ONBOARDING.md               # New developer guide
└── ONBOARDING_SUMMARY.md       # Quick reference
```

### Module Responsibilities

#### `cmd/server/`
- Application entry point
- Server initialization and startup
- Signal handling and shutdown
- Route registration and middleware setup

#### `cmd/stress-test/`
- Load testing utility for performance validation
- Simulates concurrent read/write operations

#### `internal/handlers/`
- HTTP request/response handling
- Request validation and error formatting
- API endpoint implementations:
  - `content.go` - CRUD operations for content storage
  - `health.go` - Health check and status endpoints
  - `shutdown.go` - Graceful and emergency shutdown handlers
  - `validation.go` - Request validation logic
  - `errors.go` - Centralized error handling
  - `swagger_models.go` - Swagger/OpenAPI model definitions

#### `pkg/storage/`
The storage layer is the most complex module with multiple files:

| File | Purpose |
|------|---------|
| `storage.go` | Storage interface definition (20+ methods) |
| `badger.go` | BadgerStorage struct and primary implementation |
| `badger_init.go` | Database initialization logic |
| `badger_close.go` | Shutdown and cleanup procedures |
| `badger_CRUD.go` | Create, Read, Update, Delete operations |
| `badger_queue.go` | Write queue integration for BadgerDB |
| `badger_utils.go` | Helper functions for BadgerDB operations |
| `badger_stats.go` | Statistics collection and reporting |
| `badger_size.go` | Database size calculations |
| `badger_health.go` | Health monitoring for BadgerDB |
| `badger_gc.go` | Garbage collection integration |
| `badger_backup.go` | Backup operations for BadgerDB |
| `queue.go` | Sequential write queue system with dynamic capacity |
| `backup.go` | Automated backup management and rotation |
| `gc.go` | Garbage collection with adaptive scheduling |
| `health.go` | Three-tier health monitoring (Healthy/Degraded/Unhealthy) |
| `recovery.go` | Emergency shutdown recovery procedures |
| `access_manager.go` | In-memory access count tracking |
| `pagination.go` | List pagination support |
| `filter.go` | Content filtering operations |
| `CountedSyncMap.go` | Thread-safe map with counting capabilities |
| `UnifiedNotificationSystem.go` | Event notification utilities |

#### `pkg/middleware/`
HTTP middleware components:

| File | Purpose |
|------|---------|
| `setup.go` | Middleware stack configuration and initialization |
| `auth.go` | API key authentication with safety checks |
| `throttle.go` | Custom rate limiting with backlog support |
| `echo_rate_limiter.go` | Echo framework built-in rate limiter wrapper |
| `ip_allowlist.go` | IP/CIDR-based access control |

#### `pkg/config/`
- Environment variable loading via godotenv
- Configuration validation and defaults
- Type conversion for configuration values

#### `pkg/logger/`
- Zap-based structured logging
- Production-ready configuration
- Configurable log levels per category

#### `pkg/models/`
- `Content` - Core content model with metadata
- `AccessTracker` - Access count tracking structure

#### `pkg/tls/`
- `autotls.go` - Let's Encrypt automatic certificate management

#### `static/`
- Web-based management interface
- Content browsing and management UI
- Real-time status monitoring

---

## Architecture Overview

### Write Queue System

The storage layer uses an asynchronous write queue for improved performance:

- **Single-worker processing**: All writes go through one goroutine to prevent database contention
- **Batch processing**: Writes are accumulated and processed in batches
- **Dynamic capacity**: Queue size adapts based on `WRITE_QUEUE_MAX_MULTIPLIER`
- **Configuration**:
  - `WRITE_QUEUE_SIZE` - Initial queue capacity
  - `WRITE_QUEUE_BATCH_SIZE` - Items per batch
  - `WRITE_QUEUE_BATCH_TIMEOUT` - Max wait before batch processing

### Access Tracking

Content access counts are tracked separately from persistent storage:

- **In-memory tracking**: `AccessManager` maintains counts in memory
- **Non-blocking**: Access increments don't block reads
- **Eventual consistency**: Counts sync periodically to storage

### Shutdown Hierarchy

Two shutdown modes are available:

1. **Graceful Shutdown** (`/shutdown` endpoint):
   - Stops accepting new requests
   - Drains write queue completely
   - Creates final backup
   - Closes database cleanly

2. **Emergency Shutdown** (`/emergency-shutdown` endpoint):
   - Immediate cessation
   - Emergency recovery data saved
   - Faster but may lose queued writes

### Health Monitoring

Three-tier health status system:

| Status | Meaning |
|--------|---------|
| **Healthy** | All systems operational |
| **Degraded** | Functional but with issues (e.g., queue backup) |
| **Unhealthy** | Critical failure, requires intervention |

---

## Technology Stack

| Component | Technology | Version |
|-----------|------------|---------|
| Runtime | Go | 1.24 |
| HTTP Framework | Echo | v4.13 |
| Database | BadgerDB | v4.7 |
| Logging | Zap | v1.27 |
| Swagger | swaggo | v1.16 |
| Validation | go-playground/validator | v10.26 |
| LRU Cache | hashicorp/golang-lru | v2.0.7 |
| Configuration | godotenv | v1.5.1 |

---

## Configuration Reference

Configuration is managed via environment variables, typically set in a `.env` file.

### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8081` | HTTP server port |
| `HOST` | `0.0.0.0` | Bind address |
| `READ_TIMEOUT` | `30s` | Read timeout duration |
| `WRITE_TIMEOUT` | `30s` | Write timeout duration |
| `REQUEST_TIMEOUT` | `60s` | Overall request timeout |
| `SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown deadline |
| `MAX_CONNS_PER_HOST` | `100` | Connection limit per host |

### TLS/HTTPS

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_TLS` | `false` | Enable HTTPS |
| `TLS_PORT` | `443` | HTTPS port |
| `TLS_HOSTS` | `localhost` | Certificate hostnames |
| `TLS_CACHE_DIR` | `/app/certs` | Certificate storage |
| `ENABLE_HTTPS_ONLY` | `false` | Redirect HTTP to HTTPS |

### Storage Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_DIR` | `/app/data` | Database directory |
| `BACKUP_DIR` | `/app/backups` | Backup storage location |
| `BACKUP_INTERVAL` | `6h` | Automatic backup frequency |
| `MAX_BACKUPS` | `7` | Number of backups to retain |

### Performance Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `PERFORMANCE_MODE` | `true` | Enable optimizations |
| `CACHE_SIZE` | `134217728` | LRU cache size (128MB) |
| `ENABLE_CACHE_WARMUP` | `true` | Preload cache on startup |
| `WRITE_QUEUE_SIZE` | `1000` | Initial queue capacity |
| `WRITE_QUEUE_BATCH_SIZE` | `10` | Batch processing size |
| `WRITE_QUEUE_BATCH_TIMEOUT` | `100ms` | Batch wait timeout |
| `WRITE_QUEUE_MAX_MULTIPLIER` | `10` | Max queue size multiplier |

### Security Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_AUTH` | `false` | Require API key |
| `API_KEY` | (empty) | Authentication key |
| `ALLOWED_IPS` | (empty) | IP/CIDR allowlist |
| `ALLOWED_ORIGINS` | `*` | CORS allowed origins |
| `ALLOWED_CONTENT_TYPES` | `*` | Permitted content types |
| `MAX_CONTENT_SIZE` | `10485760` | Max content size (10MB) |
| `ALLOW_QUERY_PARAM_AUTH` | `false` | Allow API key in URL (insecure) |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `THROTTLE_LIMIT` | `1000` | Custom throttle limit |
| `THROTTLE_BACKLOG_LIMIT` | `50` | Backlog queue size |
| `THROTTLE_BACKLOG_TIMEOUT` | `30s` | Backlog wait timeout |
| `ECHO_RATE_LIMIT` | `100` | Echo rate limit |
| `ECHO_BURST_LIMIT` | `50` | Echo burst allowance |
| `ECHO_RATE_LIMIT_EXPIRES_IN` | `1h` | Rate limit window |

### Garbage Collection

| Variable | Default | Description |
|----------|---------|-------------|
| `GC_INTERVAL` | `30m` | GC run frequency |
| `GC_THRESHOLD` | `0.7` | GC trigger threshold |
| `ENABLE_ADAPTIVE_GC` | `true` | Adaptive GC scheduling |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_ERROR_LOGGING` | `true` | Error level logs |
| `ENABLE_WARN_LOGGING` | `true` | Warning level logs |
| `ENABLE_INFO_LOGGING` | `true` | Info level logs |
| `ENABLE_DEBUG_LOGGING` | `false` | Debug level logs |
| `ENABLE_REQUEST_LOGGING` | `true` | HTTP request logs |
| `ENABLE_SECURITY_LOGGING` | `true` | Security event logs |

### Emergency Recovery

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_EMERGENCY_RECOVERY` | `true` | Enable recovery feature |
| `EMERGENCY_RECOVERY_DIR` | `backups/emergency-recovery` | Recovery data location |
| `EMERGENCY_TIMEOUT` | `5s` | Emergency operation timeout |

---

## API Documentation

### Swagger UI

Interactive API documentation is available at: `/swagger/index.html`

### Key Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness probe |
| `POST` | `/content` | Store content |
| `GET` | `/content/:id` | Retrieve content |
| `PUT` | `/content/:id` | Update content |
| `DELETE` | `/content/:id` | Delete content |
| `GET` | `/content` | List content (paginated) |
| `POST` | `/shutdown` | Graceful shutdown |
| `POST` | `/emergency-shutdown` | Emergency shutdown |

### Regenerating Swagger Docs

After modifying API annotations, regenerate the docs:

```bash
swag init -g cmd/server/main.go -o docs/
```

---

## Coding Standards

### Go Conventions

Follow [Effective Go](https://go.dev/doc/effective_go) and the standard Go style guide.

### Naming Conventions

```go
// Package names: lowercase, single word
package storage

// Constants: CamelCase or UPPER_CASE for exported
const MaxContentSize = 10 * 1024 * 1024

// Variables: camelCase
var writeQueueSize = 1000

// Functions: PascalCase for exported, camelCase for private
func NewBadgerStorage() *BadgerStorage { }
func processWrite() { }

// Interfaces: PascalCase, usually -er suffix
type Storage interface { }
type Reader interface { }
```

### Error Handling

```go
// Always handle errors
if err != nil {
    return fmt.Errorf("failed to initialize storage: %w", err)
}

// Use wrapped errors with context
if err := store.Save(content); err != nil {
    return fmt.Errorf("failed to save content %s: %w", content.ID, err)
}

// Create reusable error variables
var (
    ErrContentNotFound = errors.New("content not found")
    ErrInvalidInput    = errors.New("invalid input")
)
```

### Comments

```go
// Package comment describes what the package provides
package storage // Storage provides content persistence with BadgerDB

// Exported functions must have comments
// NewBadgerStorage creates a new BadgerDB-backed storage instance
// with the given options. It initializes the database, backup system,
// and garbage collection.
func NewBadgerStorage(opts BadgerOptions) (*BadgerStorage, error) {
    // ...
}

// Comment complex logic
// Process writes in batches for efficiency:
// 1. Accumulate writes until batch size or timeout
// 2. Process entire batch in single transaction
// 3. Repeat until queue is empty
for batch := range queue.batches {
    // ...
}
```

### Logging

```go
import "go.uber.org/zap"

// Use structured logging
logger.Info("Content stored",
    zap.String("id", content.ID),
    zap.String("type", content.Type),
    zap.Int("size", len(content.Data)),
)

// Log errors with context
logger.Error("Failed to store content",
    zap.String("id", content.ID),
    zap.Error(err),
)

// Use appropriate log levels
logger.Debug("Detailed debugging info")      // Development only
logger.Info("Normal operation messages")      // General information
logger.Warn("Warning messages")               // Potential issues
logger.Error("Error messages")                // Errors that don't stop execution
// logger.Fatal() - Use only in main(), calls os.Exit()
```

---

## Testing Guidelines

### Test Files

Current test files in the project:

| File | Purpose |
|------|---------|
| `pkg/middleware/auth_safety_test.go` | Auth middleware safety validation |
| `internal/handlers/errors_safety_test.go` | Error handling safety checks |
| `pkg/storage/queue_safety_test.go` | Write queue safety tests |
| `pkg/storage/queue_limits_test.go` | Queue capacity limit tests |
| `pkg/storage/recovery_safety_test.go` | Recovery system safety tests |

### Safety Test Pattern

Safety tests verify that components handle edge cases and concurrent access correctly:

```go
func TestAuthMiddleware_Safety(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"empty key", "", true},
        {"valid key", "test-key-123", false},
        {"special chars", "key!@#$%", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Running Tests

```bash
# All tests
go test ./...

# Verbose output
go test -v ./...

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test ./pkg/storage/...

# Specific test file
go test -run TestQueueSafety ./pkg/storage/

# Safety tests only
go test -run Safety ./...

# Race detection
go test -race ./...
```

### Test Helpers

Create reusable test helpers:

```go
// setupTestStore creates a test storage instance
func setupTestStore(t *testing.T) storage.Storage {
    t.Helper()

    dir := t.TempDir()
    opts := storage.BadgerOptions{
        DataDir:   filepath.Join(dir, "badger"),
        BackupDir: filepath.Join(dir, "backups"),
    }

    store, err := storage.NewBadgerStorage(opts)
    if err != nil {
        t.Fatal(err)
    }

    return store
}
```

---

## Development Workflow

### Local Development

```bash
# Start development server with hot reload (if using air)
air

# Or run directly
go run ./cmd/server

# Run with specific environment
cp .env.example .env
# Edit .env as needed
go run ./cmd/server

# Build binary
go build -o bin/server ./cmd/server

# Run tests
go test ./...

# Run linter
golangci-lint run
```

### Docker Development

```bash
# Build image
docker build -t content-storage-server .

# Run container
docker run -p 8081:8081 content-storage-server

# With docker-compose
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Swagger Documentation

```bash
# Install swag CLI (if not installed)
go install github.com/swaggo/swag/cmd/swag@latest

# Generate docs after API changes
swag init -g cmd/server/main.go -o docs/

# Access Swagger UI at http://localhost:8081/swagger/index.html
```

---

## Pull Request Process

### Before Submitting

1. **Update documentation** - Update README, comments, and docs
2. **Add tests** - Ensure new code has test coverage
3. **Run linters** - Fix any linting issues
4. **Format code** - Run `go fmt ./...`
5. **Test locally** - Verify all tests pass
6. **Regenerate Swagger** - If API changed, run `swag init`

### Branch Naming

```
feature/add-content-expiration
fix/resolve-queue-race-condition
docs/update-api-documentation
refactor/optimize-storage-layer
test/add-integration-tests
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add content expiration feature
fix: resolve race condition in access manager
docs: update README with deployment instructions
refactor: simplify queue processing logic
test: add integration tests for backup system
chore: update dependencies to latest versions
```

### Pull Request Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No new warnings generated
- [ ] Tests pass locally
- [ ] Added/updated tests for changes

## Related Issues
Fixes #123
Related to #456
```

---

## Code Review Checklist

### Functionality
- [ ] Code implements the requested feature
- [ ] Edge cases are handled
- [ ] Error handling is comprehensive
- [ ] No memory leaks or resource issues

### Code Quality
- [ ] Code is readable and understandable
- [ ] Functions are appropriately sized
- [ ] No code duplication
- [ ] Proper use of existing patterns

### Testing
- [ ] Tests cover new functionality
- [ ] Tests cover edge cases
- [ ] Tests are not flaky
- [ ] Test names are descriptive

### Documentation
- [ ] Public functions have comments
- [ ] Complex logic is explained
- [ ] README updated if needed
- [ ] Breaking changes documented

### Performance
- [ ] No performance regressions
- [ ] Efficient algorithms used
- [ ] Appropriate caching
- [ ] Resource usage reasonable

---

## Release Process

### Version Numbering

Follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible functionality
- **PATCH**: Backwards-compatible bug fixes

### Release Steps

1. **Update version** in `go.mod` if needed
2. **Update CHANGELOG** with all changes
3. **Create release tag**:
   ```bash
   git tag -a v1.2.3 -m "Release v1.2.3"
   git push origin v1.2.3
   ```
4. **Create GitHub release** with changelog
5. **Build and push Docker image**:
   ```bash
   docker build -t content-storage-server:v1.2.3 .
   docker tag content-storage-server:v1.2.3 content-storage-server:latest
   docker push content-storage-server:v1.2.3
   docker push content-storage-server:latest
   ```

### Changelog Format

```markdown
## [1.2.3] - 2025-02-10

### Added
- Content expiration feature
- Access limit enforcement

### Changed
- Improved queue processing performance
- Updated BadgerDB to v4.7.0

### Fixed
- Race condition in access manager
- Memory leak in cleanup process

### Security
- Added input sanitization for content IDs
```

---

## Getting Help

### Resources

- **[On-boarding Guide](ONBOARDING.md)** - Start here for new developers
- **[Main README](README.md)** - Project overview and usage
- **[Storage Docs](pkg/storage/README.md)** - Storage layer details
- **[Handler Docs](internal/handlers/README.md)** - API documentation

### Asking Questions

1. Check existing documentation first
2. Search existing GitHub issues
3. Create detailed issue with:
   - Steps to reproduce
   - Expected behavior
   - Actual behavior
   - Environment details

### Contributing

We welcome contributions! Please:
1. Read this development guide
2. Check existing issues for work to do
3. Follow the pull request process
4. Be patient with code review

---

**Happy coding!**
