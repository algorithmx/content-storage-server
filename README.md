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
