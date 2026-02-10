# On-boarding Guide: Content Storage Server

## Welcome! 👋

This document provides a comprehensive on-boarding guide for developers taking over the **Content Storage Server** repository. This guide will help you understand the codebase architecture, key components, and operational procedures.

---

## 📋 Table of Contents

1. [Project Overview](#project-overview)
2. [Architecture Deep Dive](#architecture-deep-dive)
3. [Quick Start Guide](#quick-start-guide)
4. [Key Components](#key-components)
5. [Development Workflow](#development-workflow)
6. [Testing Strategy](#testing-strategy)
7. [Deployment Guide](#deployment-guide)
8. [Troubleshooting](#troubleshooting)
9. [Common Tasks](#common-tasks)
10. [Resources & References](#resources--references)

---

## Project Overview

### What is Content Storage Server?

A **high-performance, enterprise-grade** content storage server built in Go that provides:

- **Asynchronous REST API** for storing and retrieving content
- **BadgerDB** embedded storage with ACID properties
- **Sequential write queue** for data consistency
- **Emergency shutdown capabilities** with automatic recovery
- **Real-time management dashboard**
- **Comprehensive health monitoring**

### Technology Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| Language | Go | 1.24+ |
| HTTP Framework | Echo | v4.13.4 |
| Database | BadgerDB | v4.7.0 |
| Logging | Zap | v1.27.0 |
| API Docs | Swagger/OpenAPI | v1.16.4 |
| Container | Docker | Alpine-based |

### Project Structure

```
content-storage-server/
├── cmd/
│   ├── server/              # Main server application
│   │   └── main.go          # Entry point with comprehensive documentation
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
│   ├── index.html
│   ├── js/management.js
│   └── css/custom.css
├── docs/                    # Auto-generated Swagger docs
├── .env                     # Environment configuration
├── .env.example             # Configuration template
├── Dockerfile               # Multi-stage build
├── docker-compose.yml       # Production deployment
└── README.md                # User documentation
```

---

## Architecture Deep Dive

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

---

## Quick Start Guide

### Prerequisites

- **Go 1.24 or later**
- **Docker** (for containerized deployment)
- **git** for version control

### Local Development Setup

```bash
# 1. Clone the repository
git clone <repository-url>
cd content-storage-server

# 2. Install dependencies
go mod download

# 3. Configure environment
cp .env.example .env
# Edit .env with your settings

# 4. Build and run
go run cmd/server/main.go

# 5. Access the server
# Management Interface: http://localhost:8081/
# API Documentation:   http://localhost:8081/swagger/index.html
# Health Check:        http://localhost:8081/health
```

### Docker Deployment

```bash
# Build and start with docker-compose
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the server
docker-compose down
```

### Verify Installation

```bash
# Store content
curl -X POST http://localhost:8081/api/v1/content/ \
  -H "Content-Type: application/json" \
  -d '{"id": "test-1", "data": "Hello, World!", "type": "text/plain"}'

# Check status
curl http://localhost:8081/api/v1/content/test-1/status

# Retrieve content
curl http://localhost:8081/api/v1/content/test-1
```

---

## Key Components

### 1. Storage Layer (`pkg/storage/`)

The core storage system with BadgerDB integration.

**Key Files:**
- `storage.go` - Storage interface definition
- `badger*.go` - BadgerDB implementation split across multiple files
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

**Important:** All handlers return structured JSON responses with consistent error handling.

### 3. Middleware (`pkg/middleware/`)

Cross-cutting concerns applied to all requests.

**Middleware Stack (in order):**
1. **Shutdown Detection** - Immediately returns 503 during shutdown
2. **Request ID** - Adds unique request tracking
3. **Rate Limiting** - Echo + custom rate limiters
4. **IP Allowlisting** - Network-level security
5. **CORS & Security Headers** - Browser security
6. **Compression** - Response compression
7. **Authentication** - API key validation (if enabled)

### 4. Configuration (`pkg/config/`)

Environment-based configuration with validation.

**Key Configuration Groups:**
- HTTP Server (port, host, timeouts)
- TLS/HTTPS (AutoTLS configuration)
- Storage (data directory, backup settings)
- Performance (caching, batching, queue size)
- Security (auth, IP restrictions, CORS)
- Rate Limiting (throttling configuration)
- Emergency Shutdown (timeouts, recovery)

### 5. Management Interface (`static/`)

Web-based dashboard for monitoring and management.

**Features:**
- Real-time health monitoring
- System metrics display
- Content management with pagination
- Manual operations (sync, backup, GC)
- Operation logging

**Architecture Note:** Fixed-height, no-scroll design using Bootstrap 5.3.0.

---

## Development Workflow

### Setting Up Development Environment

```bash
# 1. Fork and clone repository
git clone <your-fork-url>
cd content-storage-server

# 2. Create development branch
git checkout -b feature/your-feature-name

# 3. Install pre-commit hooks (if configured)
# (Check for .git/hooks pre-commit setup)

# 4. Verify installation
go mod verify
go test ./...
```

### Code Style Guidelines

- **Go Conventions:** Follow [Effective Go](https://go.dev/doc/effective_go)
- **Comments:** Document exported functions and packages
- **Error Handling:** Always handle errors, use wrapped errors with context
- **Logging:** Use structured logging with Zap

### Making Changes

1. **Identify the component** to modify (handlers, storage, middleware)
2. **Read existing code** to understand patterns
3. **Write tests** for new functionality
4. **Update documentation** (README, code comments)
5. **Test thoroughly** before committing

### Commit Guidelines

```bash
# Format your commit messages like:
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

---

## Testing Strategy

### Unit Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/storage/...

# Verbose output
go test -v ./...
```

### Functional Testing

```bash
cd cmd/functional-test
go run main.go                   # Basic validation
go run main.go --test-status     # Test async functionality
go run main.go --test-shutdown   # Test emergency shutdown
go run main.go --users 50        # Performance testing
```

### Stress Testing

```bash
cd cmd/stress-test
go run main.go -duration=60s -users=100
```

### Integration Testing

Use the management interface and Swagger docs for manual testing:

1. Start the server: `go run cmd/server/main.go`
2. Open `http://localhost:8081/swagger/index.html`
3. Test endpoints using the Swagger UI

---

## Deployment Guide

### Docker Deployment

```bash
# Build image
docker build -t content-storage-server .

# Run container
docker run -d \
  -p 8081:8081 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/backups:/app/backups \
  --env-file .env \
  content-storage-server
```

### Production Considerations

**Security Checklist:**
- [ ] Enable authentication (`ENABLE_AUTH=true`)
- [ ] Set strong API key (`API_KEY`)
- [ ] Configure IP allowlisting (`ALLOWED_IPS`)
- [ ] Enable HTTPS (`ENABLE_TLS=true`)
- [ ] Set content size limits (`MAX_CONTENT_SIZE`)

**Performance Tuning:**
- Enable `PERFORMANCE_MODE=true`
- Adjust `WRITE_QUEUE_BATCH_SIZE` and `WRITE_QUEUE_BATCH_TIMEOUT`
- Configure `THROTTLE_LIMIT` based on expected load
- Set `GC_INTERVAL` based on data patterns

**Monitoring:**
- Monitor `/health/detailed` for system health
- Use `/api/v1/metrics` for operational metrics
- Set up backup verification
- Monitor emergency shutdown logs

### Docker Compose Production

```bash
# Start with production configuration
docker-compose up -d

# Scale if needed (load balancer required)
docker-compose up -d --scale content-storage-server=3
```

---

## Troubleshooting

### Common Issues

#### 1. Storage Not Ready

**Symptom:** `storage not ready` errors

**Solution:**
- Verify BadgerDB directory exists: `ls -la data/badger/`
- Check directory permissions
- Review logs for initialization errors

#### 2. High Memory Usage

**Symptom:** Memory usage grows over time

**Solution:**
- Run garbage collection: `curl -X POST http://localhost:8081/api/v1/gc`
- Cleanup access trackers: `curl -X POST http://localhost:8081/api/v1/cleanup`
- Check cache settings in configuration

#### 3. Slow Performance

**Symptom:** High response times

**Solution:**
- Enable performance mode: `PERFORMANCE_MODE=true`
- Check queue depth: `curl http://localhost:8081/api/v1/metrics`
- Review GC settings and intervals

#### 4. Authentication Failures

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

---

## Common Tasks

### Adding a New API Endpoint

1. **Define handler** in appropriate handler file
2. **Add route** in `setupRouter()` function
3. **Add Swagger annotations** for documentation
4. **Write tests** for the endpoint
5. **Update README** with endpoint documentation

### Modifying Storage Behavior

1. **Identify relevant file** in `pkg/storage/`
2. **Update storage interface** if needed
3. **Implement changes** in BadgerDB implementation
4. **Add tests** for new behavior
5. **Consider impact** on emergency recovery

### Adding Configuration Options

1. **Add field** to `Config` struct in `pkg/config/`
2. **Add environment variable** loading
3. **Add validation** if needed
4. **Update `.env.example`** with default
5. **Document behavior** in README

### Updating Dependencies

```bash
# Update all dependencies
go get -u ./...

# Tidy up modules
go mod tidy

# Test after updates
go test ./...
```

---

## Resources & References

### Documentation

- **Main README:** `README.md` - User-facing documentation
- **Storage README:** `pkg/storage/README.md` - Storage layer details
- **Handlers README:** `internal/handlers/README.md` - API documentation
- **Static README:** `static/README.md` - Management interface docs

### External Resources

- **Go Documentation:** https://go.dev/doc/
- **Echo Framework:** https://echo.labstack.com/docs
- **BadgerDB:** https://dgraph.io/docs/badger/
- **Swagger in Go:** https://swagger.io/docs/

### Key Files to Understand First

1. `cmd/server/main.go` - Application entry point
2. `pkg/storage/storage.go` - Storage interface
3. `internal/handlers/content.go` - Content operations
4. `pkg/config/config.go` - Configuration management

### Getting Help

- **Review logs:** Check application logs for detailed error information
- **Health endpoints:** Use `/health/detailed` for system status
- **Metrics endpoint:** Use `/api/v1/metrics` for operational data
- **Existing issues:** Check GitHub issues for known problems

---

## Next Steps

1. **Read the code** - Start with `cmd/server/main.go`
2. **Run the server** - Follow the Quick Start guide
3. **Test the API** - Use Swagger UI or curl
4. **Explore the dashboard** - Open the management interface
5. **Make a small change** - Fix a bug or add a minor feature
6. **Contribute** - Submit a pull request

---

## Repository Contacts

- **License:** MIT
- **Copyright:** (c) 2025 [Yunlong Lian/algorithmx]
- **Last Updated:** 2025-02-10

---

**Welcome to the team! We're glad you're here. 🚀**
