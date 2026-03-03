# Documentation Consolidation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate three documentation files into a single, concise, user-first README with developer details moved to docs/developer-guide.md.

**Architecture:** Remove dated review metadata (ONBOARDING_SUMMARY.md), extract developer-focused content to a separate guide, and streamline README.md to ~150-200 lines focused on user needs.

**Tech Stack:** Markdown documentation, Go project

---

## Task 1: Create docs/developer-guide.md

**Files:**
- Create: `docs/developer-guide.md`

**Step 1: Create developer guide with architecture content from ONBOARDING.md**

Create the file with the developer-focused content:

```markdown
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

## Project Structure

```
content-storage-server/
├── cmd/
│   ├── server/              # Main server application
│   │   └── main.go          # Entry point
│   └── stress-test/         # Performance testing tool
├── internal/
│   └── handlers/            # HTTP request handlers
│       ├── content.go       # Content CRUD operations
│       ├── health.go        # Health & monitoring endpoints
│       ├── shutdown.go      # Shutdown coordination
│       ├── validation.go    # Request validation
│       └── swagger_models.go
├── pkg/
│   ├── config/              # Configuration management
│   ├── logger/              # Structured logging (Zap)
│   ├── middleware/          # HTTP middleware stack
│   ├── models/              # Data models
│   ├── storage/             # BadgerDB storage implementation
│   │   ├── badger*.go       # Storage implementation files
│   │   ├── queue.go         # Async write queue
│   │   ├── access_manager.go
│   │   ├── backup.go
│   │   ├── gc.go
│   │   ├── health.go
│   │   └── recovery.go      # Emergency recovery
│   └── tls/                 # TLS/HTTPS management
├── static/                  # Management web interface
├── docs/                    # Auto-generated Swagger docs
├── .env                     # Environment configuration
├── .env.example             # Configuration template
├── Dockerfile               # Multi-stage build
└── docker-compose.yml       # Production deployment
```

## Key Components

### 1. Storage Layer (`pkg/storage/`)

The core storage system with BadgerDB integration.

**Key Files:**
- `storage.go` - Storage interface definition
- `badger*.go` - BadgerDB implementation
- `queue.go` - Asynchronous write queue
- `access_manager.go` - Access count tracking
- `backup.go` - Automated backup system
- `gc.go` - Garbage collection
- `recovery.go` - Emergency shutdown recovery

**Critical Concept:** Sequential Write Queue
- Single worker processes writes sequentially
- Prevents race conditions
- Enables emergency recovery

### 2. Handler Layer (`internal/handlers/`)

HTTP request handlers for API endpoints.

**Key Files:**
- `content.go` - Content CRUD operations
- `health.go` - Health monitoring and system management
- `shutdown.go` - Graceful shutdown coordination
- `validation.go` - Request validation logic

### 3. Middleware (`pkg/middleware/`)

Cross-cutting concerns applied to all requests.

**Middleware Stack (in order):**
1. **Shutdown Detection** - Returns 503 during shutdown
2. **Request ID** - Unique request tracking
3. **Rate Limiting** - Echo + custom rate limiters
4. **IP Allowlisting** - Network-level security
5. **CORS & Security Headers** - Browser security
6. **Compression** - Response compression
7. **Authentication** - API key validation (if enabled)

## Development Workflow

### Code Style Guidelines

- **Go Conventions:** Follow [Effective Go](https://go.dev/doc/effective_go)
- **Comments:** Document exported functions and packages
- **Error Handling:** Always handle errors, use wrapped errors with context
- **Logging:** Use structured logging with Zap

### Commit Guidelines

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
```

**Step 2: Verify file was created**

Run: `head -20 docs/developer-guide.md`
Expected: First 20 lines of the new file

---

## Task 2: Rewrite README.md (Part 1 - Header, Quick Start, Features)

**Files:**
- Modify: `README.md`

**Step 1: Replace README.md with streamlined user-first content**

Write the new README.md:

```markdown
# Content Storage Server

High-performance content storage with BadgerDB, async REST API, emergency recovery, and real-time monitoring.

## Quick Start

### Prerequisites
- Go 1.24 or later

### Install & Run

```bash
git clone <repository-url>
cd content-storage-server
go mod download
go build -o server ./cmd/server
./server
```

### Access Points
- **Dashboard**: http://localhost:8081/
- **API Docs**: http://localhost:8081/swagger/index.html
- **Health**: http://localhost:8081/health

### Basic Usage

```bash
# Store content (returns HTTP 202 - queued)
curl -X POST http://localhost:8081/api/v1/content/ \
  -H "Content-Type: application/json" \
  -d '{"id": "example-1", "data": "Hello, World!", "type": "text/plain"}'

# Check status
curl http://localhost:8081/api/v1/content/example-1/status

# Retrieve content
curl http://localhost:8081/api/v1/content/example-1

# List content
curl "http://localhost:8081/api/v1/content/?limit=10&offset=0"
```

## Key Features

- **Async REST API** - HTTP 202 responses with status verification
- **BadgerDB Storage** - Fast, embedded, ACID-compliant key-value store
- **Sequential Write Queue** - Data consistency with emergency recovery
- **Emergency Shutdown** - Queue preservation and automatic restoration
- **Security** - API key auth, IP allowlisting, TLS/HTTPS
- **Rate Limiting** - Multi-layer throttling with configurable limits
- **Management Dashboard** - Real-time metrics and queue monitoring
- **Health Monitoring** - Adaptive health checks with detailed status

## Asynchronous Storage

Content storage is async for performance and safety:

1. **POST** returns **HTTP 202 Accepted** immediately
2. Content is processed sequentially and persisted
3. Use **GET /api/v1/content/{id}/status** to verify completion
4. During emergency shutdown, queued content is serialized and restored on restart

**Status Values:** `queued` | `stored` | `not_found`
```

**Step 2: Verify file was updated**

Run: `wc -l README.md`
Expected: ~60 lines so far

---

## Task 3: Rewrite README.md (Part 2 - API Reference)

**Files:**
- Modify: `README.md` (append)

**Step 1: Append API Reference section**

```markdown

## API Endpoints

### Content Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/content/` | Store content (HTTP 202) |
| GET | `/api/v1/content/` | List content with pagination |
| GET | `/api/v1/content/{id}` | Get content with access tracking |
| GET | `/api/v1/content/{id}/status` | Check storage status |
| DELETE | `/api/v1/content/{id}` | Delete content |
| GET | `/api/v1/content/count` | Get total count |

### System Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Basic health check |
| GET | `/health/detailed` | Comprehensive health (200/206/503) |
| GET | `/api/v1/metrics` | System metrics and queue stats |
| POST | `/api/v1/sync` | Manual queue flush |
| POST | `/api/v1/backup` | Create manual backup |
| POST | `/api/v1/gc` | Trigger garbage collection |
| POST | `/api/v1/cleanup` | Cleanup access trackers |

### Interface
- `GET /` - Management dashboard
- `GET /swagger/*` - API documentation
```

---

## Task 4: Rewrite README.md (Part 3 - Configuration)

**Files:**
- Modify: `README.md` (append)

**Step 1: Append Configuration section**

```markdown

## Configuration

Configure via environment variables or `.env` file.

### Core Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8081 | HTTP server port |
| `HOST` | 0.0.0.0 | Server bind address |
| `DATA_DIR` | ./data | Database directory |
| `BACKUP_DIR` | ./backups | Backup directory |

### Security
| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_AUTH` | true | API key authentication |
| `API_KEY` | "" | Authentication key (required if auth enabled) |
| `ALLOWED_IPS` | "" | IP allowlist (comma-separated) |
| `ENABLE_TLS` | false | HTTPS with automatic TLS |

### Performance
| Variable | Default | Description |
|----------|---------|-------------|
| `THROTTLE_LIMIT` | 100 | Max concurrent requests |
| `MAX_CONTENT_SIZE` | 10485760 | Max content size (10MB) |
| `WRITE_QUEUE_BATCH_SIZE` | 10 | Queue batch size |
| `WRITE_QUEUE_BATCH_TIMEOUT` | 100ms | Batch timeout |

### Emergency Shutdown
| Variable | Default | Description |
|----------|---------|-------------|
| `SHUTDOWN_TIMEOUT` | 30s | Graceful shutdown timeout |
| `EMERGENCY_TIMEOUT` | 5s | Emergency shutdown timeout |
| `ENABLE_EMERGENCY_RECOVERY` | true | Auto-recovery on startup |

See `pkg/config/config.go` for complete options.
```

---

## Task 5: Rewrite README.md (Part 4 - Testing, Production, Docs)

**Files:**
- Modify: `README.md` (append)

**Step 1: Append Testing, Production, and Documentation sections**

```markdown

## Testing

```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# Functional tests
cd cmd/functional-test
go run main.go                   # Basic validation
go run main.go --test-status     # Test async functionality
go run main.go --test-shutdown   # Test emergency shutdown

# Stress tests
cd cmd/stress-test
go run main.go -duration=60s -users=100
```

## Production Checklist

### Security
- [ ] Enable authentication (`ENABLE_AUTH=true`)
- [ ] Set strong API key (`API_KEY`)
- [ ] Configure IP allowlisting (`ALLOWED_IPS`)
- [ ] Enable HTTPS (`ENABLE_TLS=true`)
- [ ] Set content size limits (`MAX_CONTENT_SIZE`)

### Performance
- Enable `PERFORMANCE_MODE=true` for high-throughput
- Adjust `WRITE_QUEUE_BATCH_SIZE` and `WRITE_QUEUE_BATCH_TIMEOUT`
- Configure `THROTTLE_LIMIT` based on expected load

### Monitoring
- Monitor `/health/detailed` for system health
- Use `/api/v1/metrics` for operational metrics
- Watch queue depth and emergency shutdown logs

## Documentation

- **[Developer Guide](docs/developer-guide.md)** - Architecture, development workflow, troubleshooting
- **[Storage Module](pkg/storage/README.md)** - Storage layer documentation
- **[Handlers](internal/handlers/README.md)** - API handler documentation
- **[Management Interface](static/README.md)** - Dashboard documentation

## License

MIT License
```

**Step 2: Verify final README line count**

Run: `wc -l README.md`
Expected: ~150-180 lines

---

## Task 6: Delete obsolete documentation files

**Files:**
- Delete: `ONBOARDING.md`
- Delete: `ONBOARDING_SUMMARY.md`

**Step 1: Remove obsolete files**

```bash
rm ONBOARDING.md ONBOARDING_SUMMARY.md
```

**Step 2: Verify files are deleted**

Run: `ls -la *.md`
Expected: Only README.md remains in root

---

## Task 7: Commit changes

**Files:**
- Commit all changes

**Step 1: Stage all changes**

```bash
git add README.md docs/developer-guide.md
git add -u  # Stage deletions
```

**Step 2: Commit**

```bash
git commit -m "docs: consolidate documentation into single user-first README

- Streamline README.md to ~180 lines focused on user needs
- Create docs/developer-guide.md for architecture and development content
- Remove ONBOARDING.md and ONBOARDING_SUMMARY.md (dated review metadata)
- Preserve all information, reorganized by audience"
```

**Step 3: Verify commit**

Run: `git log -1 --oneline`
Expected: New commit with consolidation message
