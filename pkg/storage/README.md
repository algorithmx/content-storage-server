# Storage Module

A high-performance, reliable content storage system built on BadgerDB with advanced features for production environments.

## Overview

The storage module provides a robust, feature-rich storage layer for content management with built-in reliability, performance optimization, and operational features. It implements a clean interface while providing advanced capabilities like asynchronous writes, automatic backups, garbage collection, health monitoring, and emergency recovery.

## Key Features

- **High-Performance Storage**: BadgerDB-based with optimized read/write operations
- **Asynchronous Write Queue**: Batched writes with configurable queue size and timeouts
- **Access Tracking**: In-memory access count management with expiration support
- **Automatic Backups**: Scheduled backups with retention policies
- **Garbage Collection**: Adaptive GC scheduling with low-traffic optimization
- **Health Monitoring**: Comprehensive health checks for all components
- **Emergency Recovery**: Automatic recovery from unexpected shutdowns
- **Content Filtering**: Advanced filtering by type, tag, creation date, and expiration
- **Pagination**: Efficient pagination with filtering support
- **Graceful Shutdown**: Coordinated shutdown with timeout controls

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Storage Interface                        │
├─────────────────────────────────────────────────────────────┤
│                   BadgerStorage                             │
├─────────────────┬─────────────────┬─────────────────────────┤
│  QueuedWrite    │  AccessManager  │    BadgerDB             │
│  Batch          │                 │                         │
├─────────────────┼─────────────────┼─────────────────────────┤
│ BackupManager   │ GarbageCollector│   HealthMonitor         │
├─────────────────┼─────────────────┼─────────────────────────┤
│ Notification    │ Recovery        │   Filtering &           │
│ System          │ System          │   Pagination            │
└─────────────────┴─────────────────┴─────────────────────────┘
```

## Core Components

### BadgerStorage
The main storage implementation that orchestrates all components:
- Implements the `Storage` interface
- Manages BadgerDB instance and lifecycle
- Coordinates all subsystems (queue, backup, GC, health)
- Provides thread-safe operations

### QueuedWriteBatch
Asynchronous write system for high-throughput operations:
- Batches multiple writes for efficiency
- Provides both async (`Store`) and sync (`StoreSync`) operations
- Handles write ordering and consistency
- Supports graceful shutdown with pending write completion

### AccessManager
In-memory access tracking system:
- Tracks access counts without modifying stored content
- Supports content expiration based on access limits
- Provides atomic increment operations
- Includes cleanup mechanisms to prevent memory leaks

### BackupManager
Automated backup system:
- Scheduled backups with configurable intervals
- Backup retention with automatic cleanup
- Backup validation and integrity checks
- Support for manual backup creation

### GarbageCollector
Intelligent garbage collection:
- Adaptive scheduling based on traffic patterns
- Configurable GC thresholds
- Low-traffic hour optimization
- Comprehensive metrics tracking

### HealthMonitor
System health monitoring:
- Component health checks (database, queue, backup, GC)
- Performance metrics collection
- Health status reporting
- Degradation detection

## Usage Examples

### Basic Operations

```go
// Initialize storage
opts := storage.BadgerOptions{
    DataDir:        "data/badger",
    BackupDir:      "backups",
    BackupInterval: 6 * time.Hour,
    MaxBackups:     7,
}
store, err := storage.NewBadgerStorage(opts)
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Store content asynchronously
content := &models.Content{
    ID:   "example-1",
    Data: "Hello, World!",
    Type: "text/plain",
}
err = store.Store(content)

// Store content synchronously
err = store.StoreSync(content)

// Retrieve content with access tracking
contentWithAccess, err := store.Get("example-1")
if err != nil {
    log.Printf("Content: %s, Access Count: %d",
        contentWithAccess.Data, contentWithAccess.AccessCount)
}

// Read-only access (no access count increment)
content, err := store.GetReadOnly("example-1")
```

### Filtering and Pagination

```go
// Create filter
filter := &storage.ContentFilter{
    ContentType:    "text/plain",
    Tag:           "important",
    CreatedAfter:  &time.Now().Add(-24 * time.Hour),
    IncludeExpired: false,
}

// List with filtering and pagination
contents, err := store.ListWithFilter(10, 0, filter)

// Count matching items
count := store.CountWithFilter(filter)
```

### Access Management

```go
// Get current access count
count := store.GetAccessCount("example-1")

// Set access count (useful for restoration)
store.SetAccessCount("example-1", 5)

// Remove access tracking (when content is deleted)
store.RemoveAccessTracking("example-1")
```

## Configuration

### BadgerOptions

```go
type BadgerOptions struct {
    // Basic Configuration
    DataDir          string        // BadgerDB data directory
    BackupDir        string        // Backup storage directory

    // Backup Configuration
    BackupInterval   time.Duration // Backup frequency (default: 6h)
    MaxBackups       int           // Backup retention count (default: 7)

    // Garbage Collection
    GCInterval       time.Duration // GC frequency
    GCThreshold      float64       // GC trigger threshold (default: 0.7)
    EnableAdaptiveGC bool          // Enable adaptive GC scheduling
    LowTrafficHours  []int         // Hours for GC optimization (0-23)

    // Performance Optimization
    PerformanceMode   bool          // Enable performance optimizations
    CacheSize         int64         // In-memory cache size
    EnableCacheWarmup bool          // Cache warming on startup
    CacheTTL          time.Duration // Cache TTL (default: 5m)

    // Write Queue Configuration
    WriteQueueSize         int           // Queue buffer size (default: 10000)
    WriteQueueBatchSize    int           // Batch size for writes
    WriteQueueBatchTimeout time.Duration // Batch timeout
}
```

### Default Values

- **DataDir**: `"data/badger"`
- **BackupDir**: `"backups"`
- **BackupInterval**: `6 * time.Hour`
- **MaxBackups**: `7`
- **CacheTTL**: `5 * time.Minute`
- **WriteQueueSize**: `10000`
- **GCThreshold**: `0.7`

## Performance Features

### Asynchronous Writes
- Non-blocking write operations with queue-based batching
- Configurable batch sizes and timeouts
- Sequential ordering guarantees

### Intelligent Caching
- Configurable in-memory caching with TTL
- Cache warming for consistent performance
- Read optimization for frequently accessed content

### Adaptive Garbage Collection
- Traffic-aware GC scheduling
- Low-traffic hour optimization
- Configurable thresholds to minimize impact

### Efficient Pagination
- Stream-based iteration for large datasets
- Filter-aware pagination to minimize memory usage
- Optimized counting for filtered results

## Reliability Features

### Backup System
- Automated scheduled backups
- Configurable retention policies
- Backup integrity validation
- Manual backup creation support

### Emergency Recovery
- Automatic detection of incomplete shutdowns
- Queue state restoration from emergency files
- Pending write recovery
- Graceful degradation on recovery failures

### Health Monitoring
- Real-time component health tracking
- Performance metrics collection
- Degradation detection and reporting
- Component status verification

### Graceful Shutdown
- Coordinated shutdown of all components
- Pending write completion with timeouts
- Emergency state preservation
- Clean shutdown verification

## Error Handling

### Common Errors

```go
var (
    ErrContentNotFound   = errors.New("content not found")
    ErrContentExpired    = errors.New("content has expired")
    ErrSyncFailed        = errors.New("content synchronization failed")
    ErrStorageNotReady   = errors.New("storage not ready")
    ErrShutdownTimeout   = errors.New("graceful shutdown timeout exceeded")
    ErrComponentStillRunning = errors.New("component still running after shutdown")
)
```

### Error Recovery
- Automatic retry mechanisms for transient failures
- Graceful degradation for component failures
- Detailed error reporting for troubleshooting
- Recovery procedures for common failure scenarios

## API Reference

### Storage Interface

#### Core Operations
- `Count() int` - Get total content count
- `Store(content *models.Content) error` - Store content asynchronously
- `StoreSync(content *models.Content) error` - Store content synchronously
- `Get(id string) (*models.ContentWithAccess, error)` - Get with access tracking
- `GetReadOnly(id string) (*models.Content, error)` - Get without access tracking
- `Delete(id string) error` - Delete content

#### Access Management
- `GetAccessCount(id string) int64` - Get current access count
- `SetAccessCount(id string, count int64)` - Set access count
- `RemoveAccessTracking(id string)` - Remove access tracking

#### Listing and Filtering
- `List(limit, offset int) ([]*models.Content, error)` - List with pagination
- `ListWithFilter(limit, offset int, filter *ContentFilter) ([]*models.Content, error)` - List with filtering
- `CountWithFilter(filter *ContentFilter) int` - Count filtered items

#### Maintenance Operations
- `RunGC() error` - Run garbage collection
- `StartGCLoop(interval time.Duration)` - Start GC loop
- `StopGCLoop()` - Stop GC loop
- `FlushQueue() error` - Flush write queue

#### Lifecycle Management
- `Close() error` - Close with default timeout
- `CloseWithTimeout(timeout time.Duration) error` - Close with custom timeout
- `Restart() error` - Restart all components
- `VerifyCleanShutdown() map[string]bool` - Verify shutdown status
- `IsFullyShutdown() bool` - Check if fully shutdown
- `WaitForContentWrite(contentID string, timeout time.Duration) error` - Wait for write completion

### ContentFilter

```go
type ContentFilter struct {
    ContentType    string     // MIME type filter
    Tag           string     // Tag filter
    CreatedAfter  *time.Time // Created after timestamp
    CreatedBefore *time.Time // Created before timestamp
    IncludeExpired bool       // Include expired content
}
```

## File Structure

```
pkg/storage/
├── README.md                    # This documentation
├── storage.go                   # Main interface and types
├── badger.go                    # BadgerStorage struct and options
├── badger_init.go              # Initialization and setup
├── badger_CRUD.go              # Core CRUD operations
├── badger_close.go             # Shutdown and cleanup
├── badger_backup.go            # Backup integration
├── badger_gc.go                # Garbage collection integration
├── badger_health.go            # Health monitoring integration
├── badger_stats.go             # Statistics and metrics
├── badger_utils.go             # Utility functions
├── badger_queue.go             # Queue system integration
├── badger_size.go              # Size calculation utilities
├── queue.go                    # Asynchronous write queue
├── access_manager.go           # Access tracking system
├── backup.go                   # Backup management
├── gc.go                       # Garbage collection
├── health.go                   # Health monitoring
├── recovery.go                 # Emergency recovery
├── filter.go                   # Content filtering
├── pagination.go               # Pagination and listing
├── CountedSyncMap.go           # Thread-safe counting map
└── UnifiedNotificationSystem.go # Event notification system
```

## Best Practices

### Initialization
- Always configure backup directory and intervals for production
- Set appropriate queue sizes based on expected write volume
- Enable adaptive GC for variable traffic patterns
- Configure low-traffic hours for optimal GC scheduling

### Operations
- Use asynchronous `Store()` for high-throughput scenarios
- Use synchronous `StoreSync()` when immediate persistence is required
- Implement proper error handling for all operations
- Use read-only operations when access tracking is not needed

### Shutdown
- Always call `Close()` or `CloseWithTimeout()` for graceful shutdown
- Verify clean shutdown in critical applications
- Configure appropriate shutdown timeouts for your use case
- Monitor shutdown status in production environments

### Monitoring
- Regularly check health status for early problem detection
- Monitor GC metrics for performance optimization
- Track backup success and timing
- Monitor queue depth and processing metrics

## Troubleshooting

### Common Issues

1. **Storage Not Ready**: Ensure proper initialization before operations
2. **Shutdown Timeouts**: Increase timeout or reduce queue size
3. **High Memory Usage**: Check access manager cleanup and cache settings
4. **Slow Performance**: Review GC settings and queue configuration
5. **Backup Failures**: Verify backup directory permissions and disk space

### Debug Information

Use the health monitoring and statistics APIs to gather debug information:

```go
// Get component health status
healthStatus := store.VerifyCleanShutdown()

// Get GC statistics
gcStats := store.GetGCStats()

// Check if fully shutdown
isShutdown := store.IsFullyShutdown()
```

## Contributing

When contributing to the storage module:

1. Maintain the existing architecture and design patterns
2. Add comprehensive tests for new functionality
3. Update this documentation for any interface changes
4. Follow the established error handling patterns
5. Ensure thread safety for all operations
6. Add appropriate logging and metrics

## License

This module is part of the content storage server project and follows the same licensing terms.
