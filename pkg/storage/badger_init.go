package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// getOptimizedBadgerOptions returns performance-optimized BadgerDB options
func getOptimizedBadgerOptions(dataDir string, opts BadgerOptions) badger.Options {
	options := badger.DefaultOptions(dataDir)

	if opts.PerformanceMode {
		// OPTIMIZED configuration with minimal preallocation for stress testing
		options = options.
			// Minimal memory settings to avoid unnecessary preallocation
			WithMemTableSize(8 << 20).     // 8MB memtable (much smaller than 64MB default)
			WithValueLogFileSize(16 << 20). // 16MB value log files (much smaller than 1GB default)
			WithValueLogMaxEntries(50000).  // Fewer entries per value log
			WithBaseTableSize(4 << 20).     // 4MB SST files

			// I/O Optimization - Async writes for maximum performance
			WithSyncWrites(false). // Force async writes for maximum performance

			// Compression and Storage - Optimized for small files
			WithBlockSize(4096).          // 4KB blocks (standard size)
			WithBloomFalsePositive(0.01). // 1% false positive rate

			// Compaction optimizations for write-heavy workloads
			WithNumCompactors(2).         // Fewer compactors for smaller files
			WithNumLevelZeroTables(2).    // Fewer L0 tables for faster compaction
			WithNumLevelZeroTablesStall(4) // Lower stall threshold for aggressive compaction
		// Note: Minimal preallocation eliminates large file overhead
	} else {
		// Conservative configuration for reliability
		options = options.
			WithSyncWrites(true). // Ensure data durability
			WithNumCompactors(2)  // Conservative compaction
	}

	// Common settings
	options = options.
		WithLogger(nil).           // Disable default logger
		WithDetectConflicts(true). // Enable conflict detection
		WithNumVersionsToKeep(1)   // Keep only latest version

	// Apply cache size if specified (for read caching)
	if opts.CacheSize > 0 {
		options = options.WithBlockCacheSize(opts.CacheSize)
	}

	return options
}

// NewBadgerStorage creates a new BadgerDB-based storage with enhanced reliability features
func NewBadgerStorage(opts BadgerOptions) (*BadgerStorage, error) {
	// Set default values if not provided
	if opts.DataDir == "" {
		opts.DataDir = "data/badger"
	}
	if opts.BackupDir == "" {
		opts.BackupDir = "backups"
	}
	if opts.BackupInterval == 0 {
		opts.BackupInterval = 6 * time.Hour // Default backup every 6 hours
	}
	if opts.MaxBackups == 0 {
		opts.MaxBackups = 7 // Keep 7 backups by default
	}
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 5 * time.Minute // Default cache TTL
	}
	if opts.WriteQueueSize == 0 {
		opts.WriteQueueSize = 10000 // Default queue size for high throughput
	}
	// Single writer enforced for sequential ordering - ignore any configured value
	if opts.WriteQueueBatchSize == 0 {
		opts.WriteQueueBatchSize = 100 // Default batch size
	}
	if opts.WriteQueueBatchTimeout == 0 {
		opts.WriteQueueBatchTimeout = 10 * time.Millisecond // Default batch timeout
	}

	// Create data directory if it doesn't exist
	dataDir := filepath.Clean(opts.DataDir)

	// Get optimized BadgerDB options
	badgerOpts := getOptimizedBadgerOptions(dataDir, opts)

	db, err := badger.Open(badgerOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	// Create background operation context for proper shutdown tracking
	bgCtx, bgCancel := context.WithCancel(context.Background())

	// Create the storage
	storage := &BadgerStorage{
		db:                  db,
		backgroundOpsCtx:    bgCtx,
		backgroundOpsCancel: bgCancel,
	}

	// Initialize access manager for separate access count tracking
	storage.accessManager = NewAccessManager()

	// Initialize garbage collector with enhanced features
	storage.gc = NewGarbageCollector(db)
	storage.gc.SetStorage(storage) // Set storage reference for access manager cleanup
	if opts.GCInterval > 0 {
		storage.gc.SetInterval(opts.GCInterval)
	}
	if opts.EnableAdaptiveGC {
		storage.gc.SetAdaptiveSchedule(true)
		if len(opts.LowTrafficHours) > 0 {
			storage.gc.SetLowTrafficHours(opts.LowTrafficHours)
		}
	}

	// Configure GC options
	if opts.GCThreshold > 0 {
		storage.gc.SetGCThreshold(opts.GCThreshold)
	}

	// Initialize queued write system
	if err := storage.initializeQueuedWrites(opts); err != nil {
		// Clean up on error
		db.Close()
		return nil, fmt.Errorf("failed to initialize queued writes: %w", err)
	}

	// Initialize backup manager
	backupOpts := BackupOptions{
		BackupDir:    opts.BackupDir,
		Interval:     opts.BackupInterval,
		MaxBackups:   opts.MaxBackups,
		BackupPrefix: "badger-backup",
	}
	storage.backupManager = NewBackupManager(db, backupOpts)

	// Initialize health monitor
	storage.healthMonitor = NewHealthMonitor(storage)

	// Initialize emergency recovery preparation (for fast emergency shutdown)
	if err := storage.prepareEmergencyRecovery(opts.BackupDir); err != nil {
		// Clean up on error
		db.Close()
		return nil, fmt.Errorf("failed to prepare emergency recovery: %w", err)
	}

	// Perform immediate database optimization for consistent performance
	if opts.PerformanceMode {
		if err := storage.optimizeDatabaseOnStartup(); err != nil {
			// Log warning but don't fail startup
			// This optimization is best-effort to improve performance
		}
	}

	// Perform cache warming for consistent startup performance
	if opts.PerformanceMode && opts.EnableCacheWarmup {
		storage.warmupCache()
	}

	return storage, nil
}

// NewBadgerStorageWithRecovery creates a new BadgerDB storage instance with emergency recovery option
func NewBadgerStorageWithRecovery(opts BadgerOptions, enableRecovery bool) (*BadgerStorage, error) {
	storage, err := NewBadgerStorage(opts)
	if err != nil {
		return nil, err
	}

	// Perform emergency recovery if enabled
	if enableRecovery {
		if err := storage.RecoverFromEmergencyShutdown(); err != nil {
			// Log error but don't fail startup - recovery is best-effort
			fmt.Printf("Emergency recovery failed: %v\n", err)
		}
	}

	return storage, nil
}

// initializeQueuedWrites sets up the asynchronous queued write system
func (s *BadgerStorage) initializeQueuedWrites(opts BadgerOptions) error {
	queueOpts := QueuedWriteBatchOptions{
		QueueSize:    opts.WriteQueueSize,
		BatchSize:    opts.WriteQueueBatchSize,
		BatchTimeout: opts.WriteQueueBatchTimeout,
		ErrorHandler: nil, // Use default error handler
	}

	s.queuedBatch = NewQueuedWriteBatch(s.db, queueOpts)
	return nil
}

// prepareEmergencyRecovery pre-initializes emergency recovery infrastructure for fast shutdown
func (s *BadgerStorage) prepareEmergencyRecovery(backupDir string) error {
	// Pre-create emergency recovery directory structure
	s.emergencyRecoveryDir = filepath.Join(backupDir, "emergency-recovery")

	if err := os.MkdirAll(s.emergencyRecoveryDir, 0755); err != nil {
		return fmt.Errorf("failed to create emergency recovery directory: %w", err)
	}

	// Pre-prepare minimal metadata template (only essential recovery information)
	s.emergencyMetadataTemplate = map[string]interface{}{
		"type": "emergency",
	}

	// Pre-allocate buffer for metadata JSON (estimated max size: 512 bytes)
	s.emergencyMetadataBuffer = make([]byte, 0, 512)

	return nil
}

// warmupCache performs cache warming to improve first-launch performance
func (s *BadgerStorage) warmupCache() {
	// Perform aggressive cache warming to reduce cold start performance gap
	// This helps reduce the performance difference between cold and warm starts

	// Phase 1: Warm up with key iteration (with context cancellation support)
	go func() {

		err := s.db.View(func(txn *badger.Txn) error {
			// Check if we should cancel
			select {
			case <-s.backgroundOpsCtx.Done():
				return s.backgroundOpsCtx.Err()
			default:
			}

			opts := badger.DefaultIteratorOptions
			opts.PrefetchSize = 500 // More aggressive prefetching
			opts.PrefetchValues = true // Prefetch values to warm value cache too

			iterator := txn.NewIterator(opts)
			defer iterator.Close()

			count := 0
			maxWarmupItems := 2000 // More items for better warming

			for iterator.Rewind(); iterator.Valid() && count < maxWarmupItems; iterator.Next() {
				// Check for cancellation periodically
				if count%100 == 0 {
					select {
					case <-s.backgroundOpsCtx.Done():
						return s.backgroundOpsCtx.Err()
					default:
					}
				}

				// Access both key and value to warm caches
				item := iterator.Item()
				item.Key() // Warm key cache
				item.ValueCopy(nil) // Warm value cache (ignore errors)
				count++
			}

			return nil
		})

		if err != nil && err != context.Canceled {
			fmt.Printf("=== BADGER STORAGE: Cache warmup Phase 1 error: %v ===\n", err)
		} else {
			// fmt.Printf("=== BADGER STORAGE: Cache warmup Phase 1 completed ===\n")
		}
	}()

	// Phase 2: Perform some sample operations to warm operational caches
	go func() {

		// Simulate typical operations to warm up internal caches
		for i := 0; i < 10; i++ {
			// Check for cancellation
			select {
			case <-s.backgroundOpsCtx.Done():
				// fmt.Printf("=== BADGER STORAGE: Cache warmup Phase 2 cancelled ===\n")
				return
			default:
			}

			// Perform sample reads to warm read paths
			s.db.View(func(txn *badger.Txn) error {
				// Try to read some keys that might exist
				testKey := fmt.Sprintf("warmup-key-%d", i)
				txn.Get([]byte(testKey))
				// Ignore errors - this is just for cache warming
				return nil
			})
		}

	}()
}

// optimizeDatabaseOnStartup performs immediate database optimizations to improve startup performance
func (s *BadgerStorage) optimizeDatabaseOnStartup() error {
	// Trigger immediate compaction to optimize file structure
	// This helps reduce the performance difference between cold and warm starts
	go func() {
		defer func() {
			// Ensure we handle any panics from BadgerDB operations
			if r := recover(); r != nil {
				// Log the panic but don't crash the application
				fmt.Printf("Background optimization panic recovered: %v\n", r)
			}
		}()

		// Run aggressive compaction in background to avoid blocking startup
		// This will reorganize data for better performance immediately

		// First, run value log GC to clean up space (reduced iterations for faster shutdown)
		for i := 0; i < 2; i++ { // Reduced from 5 to 2 iterations
			// Check for cancellation before each operation
			select {
			case <-s.backgroundOpsCtx.Done():
				fmt.Println("Background GC cancelled")
				return
			default:
			}

			// Run GC with timeout to prevent hanging
			gcDone := make(chan error, 1)
			go func() {
				gcDone <- s.db.RunValueLogGC(0.5) // Higher threshold for faster completion
			}()

			select {
			case err := <-gcDone:
				if err != nil {
					// No more GC needed or error occurred
					break
				}
			case <-time.After(2 * time.Second):
				// GC timeout - stop trying
				fmt.Println("Background GC timeout")
				break
			case <-s.backgroundOpsCtx.Done():
				fmt.Println("Background GC cancelled during operation")
				return
			}
		}

		// Skip LSM tree compaction as it can be very slow and interfere with shutdown
		// The database will handle compaction automatically as needed
		fmt.Println("Background optimization completed")

	}()

	return nil
}
