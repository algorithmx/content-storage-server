# Content Storage Server

A high-performance, enterprise-grade content storage server built with Go, featuring BadgerDB storage, emergency shutdown capabilities, and comprehensive management tools.

## 🚀 Key Features

- **Asynchronous REST API** - HTTP 202 responses with status verification endpoints
- **BadgerDB Backend** - Fast, embedded LSM-tree key-value store with ACID properties
- **Sequential Write Queue** - Ensures data consistency and supports emergency recovery
- **Emergency Shutdown** - Catastrophic failure handling with queue serialization and recovery
- **Authentication & Security** - API key auth, IP allowlisting, TLS/HTTPS support
- **Rate Limiting** - Multi-layer throttling with configurable limits
- **Real-time Management** - Web dashboard with live metrics and queue monitoring
- **Comprehensive Testing** - Functional, stress, and emergency shutdown test suites

## 🏃 Quick Start

### Prerequisites
- Go 1.24 or later

### Installation & Setup

```bash
# Clone and build
git clone <repository-url>
cd content-host
go mod download
go build cmd/server

# Run the server
./server
```

### Access Points
- **Management Dashboard**: http://localhost:8081/
- **API Documentation**: http://localhost:8081/swagger/index.html
- **Health Check**: http://localhost:8081/health

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

## 🔄 Asynchronous Storage

Content storage is asynchronous for performance and emergency safety:

1. **POST** returns **HTTP 202 Accepted** immediately after queuing
2. Content is processed sequentially and persisted to disk
3. Use **GET /api/v1/content/{id}/status** to verify storage completion
4. During emergency shutdown, queued content is serialized and restored on restart

**Status Values:** `queued` | `stored` | `not_found`

## 📚 API Endpoints

### Content Management
- `POST /api/v1/content/` - Store content (HTTP 202 - queued)
- `GET /api/v1/content/` - List content with pagination
- `GET /api/v1/content/{id}` - Get content with access tracking
- `GET /api/v1/content/{id}/status` - Check storage status
- `DELETE /api/v1/content/{id}` - Delete content
- `GET /api/v1/content/count` - Get total count

### System Management
- `GET /health` - Basic health check with storage connectivity
- `GET /health/detailed` - Comprehensive health with adaptive status codes (200/206/503)
- `GET /api/v1/metrics` - System metrics including queue performance and health status
- `POST /api/v1/sync` - Manual queue flush and database synchronization
- `POST /api/v1/backup` - Create manual backup with comprehensive monitoring
- `POST /api/v1/gc` - Trigger garbage collection with performance tracking
- `POST /api/v1/cleanup` - Cleanup access trackers to prevent memory leaks

### Interface
- `GET /` - Management dashboard
- `GET /swagger/*` - API documentation

## ⚙️ Configuration

Configure via environment variables or `.env` file:

### Core Settings
- `PORT=8081` - HTTP server port
- `HOST=0.0.0.0` - Server bind address
- `DATA_DIR=./data` - Database directory
- `BACKUP_DIR=./backups` - Backup directory

### Security
- `ENABLE_AUTH=false` - API key authentication
- `API_KEY=""` - Authentication key
- `ALLOWED_IPS=""` - IP allowlist (comma-separated)
- `ENABLE_TLS=false` - HTTPS with automatic TLS

### Performance
- `THROTTLE_LIMIT=100` - Max concurrent requests
- `MAX_CONTENT_SIZE=10485760` - Max content size (10MB)
- `WRITE_QUEUE_BATCH_SIZE=10` - Queue batch size
- `WRITE_QUEUE_BATCH_TIMEOUT=100ms` - Batch timeout

### Emergency Shutdown
- `SHUTDOWN_TIMEOUT=30s` - Graceful shutdown timeout
- `EMERGENCY_TIMEOUT=5s` - Emergency shutdown timeout
- `ENABLE_EMERGENCY_RECOVERY=true` - Auto-recovery on startup
- `EMERGENCY_RECOVERY_DIR=backups/emergency-recovery` - Recovery directory

### Maintenance
- `BACKUP_INTERVAL=6h` - Auto-backup frequency
- `GC_INTERVAL=30m` - Garbage collection frequency
- `MAX_BACKUPS=10` - Backup retention count

See `pkg/config/config.go` for complete options.

## 🖥️ Management Interface

Web dashboard at `http://localhost:8081/` provides:

- **Real-time Monitoring** - Server status, uptime, content count, queue depth
- **System Metrics** - Database size, backup statistics, performance data
- **Content Management** - Browse and manage stored content with pagination
- **Operations** - Trigger sync, backups, garbage collection
- **Auto-refresh** - 30-second updates, responsive design

## 🧪 Testing

### Unit Tests
```bash
go test ./...                    # All tests
go test -cover ./...             # With coverage
go test ./pkg/storage/...        # Storage tests
```

### Functional Testing
```bash
cd cmd/functional-test
go run main.go                   # Basic validation
go run main.go --test-status     # Test async functionality
go run main.go --test-shutdown   # Test emergency shutdown
go run main.go --users 50        # Performance testing
```

### Emergency Shutdown Tests
```bash
go test ./pkg/storage -run "Test.*Emergency" -v
```

### Stress Testing
```bash
cd cmd/stress-test
go run main.go -duration=60s -users=100
```

## 🏗️ Architecture

### Storage Layer
- **BadgerDB** - Embedded LSM-tree key-value store with ACID properties
- **Write Queue** - Sequential processing with batching and emergency serialization
- **Emergency Recovery** - Queue state preservation and automatic restoration
- **Access Manager** - Atomic access count tracking with CountedSyncMap

### API Layer
- **Echo Framework** - High-performance HTTP router with middleware
- **Asynchronous Handlers** - Content management with status tracking
- **Health Monitoring** - Adaptive health checks with detailed system information
- **Swagger Integration** - Automatic API documentation

### Queue System
- **Sequential Processing** - Ensures data consistency, prevents race conditions
- **Batch Processing** - Configurable batch size and timeout
- **Emergency Serialization** - Preserves pending operations during failures
- **Recovery Mechanism** - Automatic detection and restoration of emergency backups

## 🚨 Emergency Shutdown

### Shutdown Types
- **Graceful** (single Ctrl+C): Waits for pending operations (30s timeout)
- **Emergency** (double Ctrl+C or SIGUSR1): Immediate shutdown with queue preservation (5s timeout)

### Emergency Process
1. **Immediate Queue Stop** - Terminates worker without waiting
2. **Queue Serialization** - Saves pending items and write queue to JSON
3. **Recovery Metadata** - Records shutdown info for automatic recovery

### Recovery
- **Automatic** - Server detects and restores emergency files on startup
- **File Structure** - `backups/emergency-recovery/` with timestamped files
- **Archival** - Processed files moved to `processed/` subdirectory

### Usage
```bash
# Normal shutdown
kill -TERM <pid>

# Emergency shutdown
kill -USR1 <pid>
# OR double Ctrl+C within 3 seconds
```

## 🚨 Production Considerations

### Security Checklist
- [ ] Enable authentication (`ENABLE_AUTH=true`)
- [ ] Set strong API key (`API_KEY`)
- [ ] Configure IP allowlisting (`ALLOWED_IPS`)
- [ ] Enable HTTPS (`ENABLE_TLS=true`)
- [ ] Set content size limits (`MAX_CONTENT_SIZE`)

### Performance Tuning
- Enable `PERFORMANCE_MODE=true` for high-throughput
- Adjust `WRITE_QUEUE_BATCH_SIZE` and `WRITE_QUEUE_BATCH_TIMEOUT`
- Configure `THROTTLE_LIMIT` based on expected load
- Set `GC_INTERVAL` based on data patterns

### Monitoring
- Monitor `/health/detailed` for adaptive system health
- Use `/api/v1/metrics` for operational metrics
- Watch emergency shutdown logs and queue depth
- Set up backup verification and retention monitoring

## 📊 Metrics & Monitoring

### Available Metrics (`/api/v1/metrics`)
- **Content Statistics** - Total count, database size
- **Performance** - Request rates, response times, throughput
- **Storage Health** - Database health, disk usage
- **Queue Metrics** - Write queue depth, processing rate, batch statistics
- **System Health** - Overall status (healthy/degraded/unhealthy)

### Health Status Codes (`/health/detailed`)
- **200 OK** - All systems healthy
- **206 Partial Content** - System degraded but functional
- **503 Service Unavailable** - System unhealthy

### Example Response
```json
{
  "content_count": 1250,
  "database_size_bytes": 52428800,
  "health_status": "healthy",
  "queue_depth": 3,
  "queue_processing_rate": 45.2
}
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Add comprehensive tests for new functionality
4. Ensure all tests pass (`go test ./...`)
5. Submit a pull request

## 📞 Support

- **API Documentation**: http://localhost:8081/swagger/index.html
- **Management Interface**: http://localhost:8081/
- **Health Monitoring**: http://localhost:8081/health/detailed

## 📄 License

MIT License - see LICENSE file for details.
