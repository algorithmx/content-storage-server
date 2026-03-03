# Developer Guide

Architecture, development workflow, and troubleshooting for contributors.

## Architecture

### Design Patterns

1. **Clean Architecture**: Clear separation between handlers, business logic, and data access
2. **Sequential Write Queue**: Single-writer pattern ensures data consistency
3. **Middleware Stack**: Ordered middleware pipeline for cross-cutting concerns
4. **Emergency Recovery**: State preservation for catastrophic failures

### Request Flow

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP Request
       ▼
┌─────────────────────────────────────────┐
│         Middleware Stack                 │
│  1. Shutdown Detection                   │
│  2. Rate Limiting                        │
│  3. Security Headers & CORS              │
│  4. Compression                          │
│  5. Authentication (if enabled)          │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│         Handler Layer                    │
│  - ContentHandler (CRUD operations)      │
│  - HealthHandler (monitoring)            │
│  - ShutdownHandler (coordination)        │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│         Storage Layer                    │
│  - BadgerDB (persistent storage)         │
│  - Write Queue (async processing)        │
│  - Access Manager (in-memory tracking)   │
└─────────────────────────────────────────┘
```

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Sequential Write Queue** | Prevents race conditions, ensures data consistency |
| **Asynchronous Processing** | High throughput with immediate HTTP 202 responses |
| **BadgerDB** | Embedded, no external dependencies, ACID compliant |
| **Emergency Recovery** | Preserves queue state during catastrophic failures |
| **Single Writer Pattern** | Simplifies concurrency, prevents write conflicts |

### Write Queue System

The storage layer uses an asynchronous write queue for improved performance:

- **Single-worker processing**: All writes go through one goroutine to prevent database contention
- **Batch processing**: Writes are accumulated and processed in batches
- **Dynamic capacity**: Queue size adapts based on `WRITE_QUEUE_MAX_MULTIPLIER`
- **Configuration**:
  - `WRITE_QUEUE_SIZE` - Initial queue capacity
  - `WRITE_QUEUE_BATCH_SIZE` - Items per batch
  - `WRITE_QUEUE_BATCH_TIMEOUT` - Max wait before batch processing

### Health Monitoring

Three-tier health status system:

| Status | Meaning |
|--------|---------|
| **Healthy** | All systems operational |
| **Degraded** | Functional but with issues (e.g., queue backup) |
| **Unhealthy** | Critical failure, requires intervention |

## Project Structure

```
content-storage-server/
├── cmd/
│   ├── server/              # Main server application
│   │   └── main.go          # Entry point with Echo v4 setup
│   └── stress-test/         # Load testing tool
├── internal/
│   └── handlers/            # HTTP request handlers
│       ├── content.go       # Content CRUD operations
│       ├── health.go        # Health & monitoring endpoints
│       ├── shutdown.go      # Graceful/emergency shutdown
│       ├── validation.go    # Request validation
│       ├── errors.go        # Error handling utilities
│       └── swagger_models.go
├── pkg/
│   ├── config/              # Configuration management
│   ├── logger/              # Structured logging (Zap)
│   ├── middleware/          # HTTP middleware stack
│   ├── models/              # Data models
│   ├── storage/             # BadgerDB storage implementation
│   │   ├── storage.go       # Storage interface definition
│   │   ├── badger.go        # BadgerStorage struct
│   │   ├── badger_*.go      # BadgerDB implementation files
│   │   ├── queue.go         # Async write queue
│   │   ├── access_manager.go
│   │   ├── backup.go
│   │   ├── gc.go
│   │   ├── health.go
│   │   ├── recovery.go      # Emergency recovery
│   │   ├── pagination.go    # List pagination
│   │   ├── filter.go        # Content filtering
│   │   ├── CountedSyncMap.go
│   │   └── UnifiedNotificationSystem.go
│   └── tls/                 # TLS/HTTPS management
├── static/                  # Management web interface
├── docs/                    # Auto-generated Swagger docs
├── .env.example             # Configuration template
├── Dockerfile               # Multi-stage build
└── docker-compose.yml       # Production deployment
```

## Key Components

### 1. Storage Layer (`pkg/storage/`)

The core storage system with BadgerDB integration.

**Key Files:**
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
| `queue.go` | Sequential write queue system |
| `backup.go` | Automated backup management and rotation |
| `gc.go` | Garbage collection with adaptive scheduling |
| `health.go` | Three-tier health monitoring |
| `recovery.go` | Emergency shutdown recovery procedures |
| `access_manager.go` | In-memory access count tracking |
| `pagination.go` | List pagination support |
| `filter.go` | Content filtering operations |
| `CountedSyncMap.go` | Thread-safe map with counting |
| `UnifiedNotificationSystem.go` | Event notification utilities |

**Critical Concept:** Sequential Write Queue
- Single worker processes writes sequentially
- Prevents race conditions
- Enables emergency recovery

### 2. Handler Layer (`internal/handlers/`)

HTTP request handlers for API endpoints.

**Key Files:**
| File | Purpose |
|------|---------|
| `content.go` | Content CRUD operations |
| `health.go` | Health monitoring and system management |
| `shutdown.go` | Graceful and emergency shutdown handlers |
| `validation.go` | Request validation logic |
| `errors.go` | Centralized error handling |
| `swagger_models.go` | Swagger/OpenAPI model definitions |

### 3. Middleware (`pkg/middleware/`)

HTTP middleware components:

| File | Purpose |
|------|---------|
| `setup.go` | Middleware stack configuration and initialization |
| `auth.go` | API key authentication with safety checks |
| `throttle.go` | Custom rate limiting with backlog support |
| `echo_rate_limiter.go` | Echo framework built-in rate limiter wrapper |
| `ip_allowlist.go` | IP/CIDR-based access control |

**Middleware Stack (in order):**
1. **Shutdown Detection** - Returns 503 during shutdown
2. **Request ID** - Unique request tracking
3. **Rate Limiting** - Echo + custom rate limiters
4. **IP Allowlisting** - Network-level security
5. **CORS & Security Headers** - Browser security
6. **Compression** - Response compression
7. **Authentication** - API key validation (if enabled)

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

## Development Workflow

### Initial Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/content-storage-server.git
cd content-storage-server

# Install dependencies
go mod download

# Run tests to verify setup
go test ./...

# Copy and configure environment
cp .env.example .env
# Edit .env as needed
```

### Recommended Tools

- **GoLand** or **VSCode** with Go extension
- **dlv** for debugging (`go install github.com/go-delve/delve/cmd/dlv@latest`)
- **golangci-lint** for linting
- **swag** for Swagger doc generation (`go install github.com/swaggo/swag/cmd/swag@latest`)

### Local Development

```bash
# Run directly
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
docker-compose logs -f
docker-compose down
```

### Swagger Documentation

```bash
# Generate docs after API changes
swag init -g cmd/server/main.go -o docs/

# Access Swagger UI at http://localhost:8081/swagger/index.html
```

## Testing Guidelines

### Test Files

| File | Purpose |
|------|---------|
| `pkg/middleware/auth_safety_test.go` | Auth middleware safety validation |
| `internal/handlers/errors_safety_test.go` | Error handling safety checks |
| `pkg/storage/queue_safety_test.go` | Write queue safety tests |
| `pkg/storage/queue_limits_test.go` | Queue capacity limit tests |
| `pkg/storage/recovery_safety_test.go` | Recovery system safety tests |

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

# Safety tests only
go test -run Safety ./...

# Race detection
go test -race ./...
```

## Coding Standards

### Go Conventions

Follow [Effective Go](https://go.dev/doc/effective_go) and standard Go style.

### Error Handling

```go
// Always handle errors with context
if err != nil {
    return fmt.Errorf("failed to initialize storage: %w", err)
}

// Create reusable error variables
var (
    ErrContentNotFound = errors.New("content not found")
    ErrInvalidInput    = errors.New("invalid input")
)
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
```

### Commit Guidelines

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```bash
git commit -m "feat: add content expiration feature"
git commit -m "fix: resolve race condition in access manager"
git commit -m "docs: update README with deployment instructions"
```

**Commit Types:**
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `refactor:` Code refactoring
- `test:` Test updates
- `chore:` Maintenance tasks

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

## Troubleshooting

### Storage Not Ready

**Symptom:** `storage not ready` errors

**Solution:**
- Verify BadgerDB directory: `ls -la data/badger/`
- Check directory permissions
- Review logs for initialization errors

### High Memory Usage

**Symptom:** Memory usage grows over time

**Solution:**
- Run garbage collection: `curl -X POST http://localhost:8081/api/v1/gc`
- Cleanup access trackers: `curl -X POST http://localhost:8081/api/v1/cleanup`
- Check cache settings in configuration

### Slow Performance

**Symptom:** High response times

**Solution:**
- Enable performance mode: `PERFORMANCE_MODE=true`
- Check queue depth: `curl http://localhost:8081/api/v1/metrics`
- Review GC settings and intervals

### Authentication Failures

**Symptom:** HTTP 401 errors

**Solution:**
- Verify API key is set correctly
- Check `ENABLE_AUTH` setting
- Ensure `X-API-Key` header is sent

### Debug Mode

Enable profiler for debugging:

```bash
# In .env file
ENABLE_PROFILER=true

# Access profiler
http://localhost:8081/debug/pprof/
```

## Common Tasks

### Adding a New API Endpoint

1. Define handler in appropriate handler file
2. Add route in `setupRouter()` function
3. Add Swagger annotations for documentation
4. Write tests for the endpoint
5. Update README with endpoint documentation

### Modifying Storage Behavior

1. Identify relevant file in `pkg/storage/`
2. Update storage interface if needed
3. Implement changes in BadgerDB implementation
4. Add tests for new behavior
5. Consider impact on emergency recovery

### Adding Configuration Options

1. Add field to `Config` struct in `pkg/config/`
2. Add environment variable loading
3. Add validation if needed
4. Update `.env.example` with default
5. Document behavior in README

## Resources

- **Go Documentation:** https://go.dev/doc/
- **Echo Framework:** https://echo.labstack.com/docs
- **BadgerDB:** https://dgraph.io/docs/badger/
- **Swagger in Go:** https://swagger.io/docs/
