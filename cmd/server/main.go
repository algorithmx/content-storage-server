// Package main provides the Content Storage Server with comprehensive API documentation.
//
// @title Content Storage Server API
// @version 1.0
// @description A high-performance content storage server with enterprise features including expiration, access limits, backup management, and sequential write processing.
// @description
// @description ## Configuration Impact on API Behavior
// @description
// @description The server behavior is heavily influenced by environment configuration:
// @description
// @description ### Authentication (ENABLE_AUTH)
// @description - When `ENABLE_AUTH=true`: All API endpoints except `/health`, `/health/detailed`, `/ping`, and `/debug/*` require authentication
// @description - Authentication methods: `X-API-Key` header or `api_key` query parameter
// @description - When `ENABLE_AUTH=false`: All endpoints are publicly accessible
// @description
// @description ### Content Size Limits (MAX_CONTENT_SIZE)
// @description - Default: 10MB (10485760 bytes)
// @description - All content storage operations validate against this limit
// @description - Exceeding this limit returns HTTP 400 with validation error
// @description
// @description ### Rate Limiting (THROTTLE_LIMIT, THROTTLE_BACKLOG_LIMIT)
// @description - `THROTTLE_LIMIT`: Maximum concurrent requests (default: 1000)
// @description - `THROTTLE_BACKLOG_LIMIT`: Maximum queued requests (default: 50)
// @description - Exceeding limits returns HTTP 429 Too Many Requests
// @description
// @description ### Content Type Restrictions (ALLOWED_CONTENT_TYPES)
// @description - Default: `application/json,text/plain`
// @description - Only specified content types are accepted for requests
// @description - Invalid content types return HTTP 415 Unsupported Media Type
// @description
// @description ### Compression (ENABLE_COMPRESSION, COMPRESSION_LEVEL)
// @description - When enabled, responses are compressed based on Accept-Encoding header
// @description - Compression level affects CPU usage vs bandwidth trade-off
// @description
// @description ### Backup & Storage Reliability
// @description - `BACKUP_INTERVAL`: Automated backup frequency (default: 6h)
// @description - `GC_INTERVAL`: Garbage collection frequency (default: 5m)
// @description - `PERFORMANCE_MODE`: Enable performance optimizations (default: true)
// @description - Sequential write processing ensures data consistency and emergency shutdown capabilities
// @description
// @description ### Emergency Shutdown & Recovery
// @description - `ENABLE_EMERGENCY_RECOVERY`: Enable automatic recovery from emergency shutdowns (default: true)
// @description - Emergency shutdown preserves volatile queue state for automatic recovery on restart
// @description - Graceful shutdown (SIGTERM, single Ctrl+C): Waits for pending operations with 30s timeout
// @description - Emergency shutdown (SIGUSR1, double Ctrl+C): Immediate termination with state preservation
// @description - Recovery files stored in `backups/emergency-recovery/` with automatic archival after processing
// @description
// @description ### Write Queue System
// @description - `WRITE_QUEUE_SIZE`: Maximum queue capacity (default: 1000, auto-doubles when full)
// @description - `WRITE_QUEUE_BATCH_SIZE`: Items processed per batch (default: 10)
// @description - `WRITE_QUEUE_BATCH_TIMEOUT`: Maximum wait before processing partial batch (default: 100ms)
// @description - Sequential processing prevents race conditions and ensures data consistency
// @description - Queue metrics available via `/api/v1/metrics` endpoint for monitoring
// @description
// @termsOfService https://example.com/terms/
// @contact.name API Support
// @contact.url https://example.com/support
// @contact.email support@example.com
// @license.name MIT
// @license.url https://opensource.org/licenses/MIT
// @host localhost:8081
// @BasePath /
// @schemes http https
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API key authentication. Required when ENABLE_AUTH=true. Can also be provided as 'api_key' query parameter.
//
// @securityDefinitions.apikey ApiKeyQuery
// @in query
// @name api_key
// @description API key authentication via query parameter. Alternative to X-API-Key header.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"content-storage-server/internal/handlers"
	"content-storage-server/pkg/config"
	"content-storage-server/pkg/logger"
	custommiddleware "content-storage-server/pkg/middleware"
	"content-storage-server/pkg/storage"
	tls "content-storage-server/pkg/tls"

	"github.com/labstack/echo/v4"
	echoSwagger "github.com/swaggo/echo-swagger"
	"go.uber.org/zap"

	_ "content-storage-server/docs" // Import generated docs
)



// main is the primary entry point for the Content Storage Server.
//
// This function orchestrates the complete server lifecycle including initialization,
// startup, operation, and shutdown with comprehensive error handling and monitoring.
//
// ## Initialization Sequence:
// 1. **Logger Setup** - Initializes structured logging with environment configuration
// 2. **Configuration Loading** - Loads and validates configuration from environment
// 3. **Directory Creation** - Creates necessary directories for data, backups, logs
// 4. **Storage Initialization** - Initializes BadgerDB with performance optimizations
// 5. **Handler Setup** - Creates HTTP handlers for content and health management
// 6. **Router Configuration** - Sets up Echo router with middleware and routes
// 7. **Server Startup** - Starts HTTP/HTTPS server with shutdown handling
//
// ## Component Integration:
// - **BadgerDB Storage**: High-performance embedded database with ACID properties
// - **Sequential Write Queue**: Ensures data consistency and emergency recovery
// - **Health Monitoring**: Continuous system health assessment and reporting
// - **Backup System**: Automated backup creation and management
// - **Emergency Recovery**: Automatic detection and recovery from emergency shutdowns
//
// ## Server Modes:
// - **HTTP Mode**: Standard HTTP server on configured host:port
// - **HTTPS Mode**: AutoTLS with Let's Encrypt certificate management
// - **HTTPS-Only Mode**: Redirects HTTP traffic to HTTPS
//
// ## Shutdown Handling:
// - **Graceful Shutdown**: Single Ctrl+C or SIGTERM - waits for pending operations
// - **Emergency Shutdown**: Double Ctrl+C or SIGUSR1 - immediate with state preservation
// - **Request Refusal**: Immediately refuses new requests upon any shutdown signal
//
// ## Configuration Dependencies:
// - Environment variables and .env file for configuration
// - Required directories: data/, backups/, logs/
// - Optional TLS certificate cache directory
//
// ## Error Handling:
// - **Fatal Errors**: Logger initialization, storage setup failures
// - **Warning Errors**: Backup system failures, directory creation issues
// - **Graceful Degradation**: Continues operation with reduced functionality where possible
//
// ## Monitoring Integration:
// - Comprehensive startup logging with system information
// - Health monitoring system for operational status
// - Performance profiler endpoints (when enabled)
// - Management dashboard for real-time monitoring
//
// The server runs indefinitely until terminated by signal or fatal error.
func main() {
	// Print startup banner with server identification
	printStartupBanner()

	// Initialize structured logger with environment-based configuration
	appLogger, err := logger.CreateLoggerFromEnv()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer appLogger.Sync()

	appLogger.Info("Starting Content Storage Server")

	// Load configuration from environment variables and .env file
	cfg := config.Load()

	// Create necessary directories for data storage, backups, and logs
	createDirectories(cfg)

	// Display current configuration for operational visibility
	cfg.DisplayConfiguration()

	// Log comprehensive server configuration and system status
	logComprehensiveStartupInfo(appLogger, cfg)

	// Initialize BadgerDB storage with performance optimizations and reliability features
	store, err := initializeStorage(cfg, appLogger)
	if err != nil {
		appLogger.Fatal("Failed to initialize storage", zap.Error(err))
	}
	defer store.Close()

	// Initialize shutdown handler for coordinated shutdown management
	shutdownHandler := handlers.NewShutdownHandler(appLogger)

	// Initialize HTTP request handlers for content and health management
	contentHandler := handlers.NewContentHandler(store, appLogger, cfg)
	healthHandler := handlers.NewHealthHandler(store, appLogger)

	// Setup Echo router with middleware stack and route definitions
	router := setupRouter(cfg, contentHandler, healthHandler, shutdownHandler, appLogger)

	// Log router and middleware configuration
	appLogger.Info("HTTP Router Configuration",
		zap.String("framework", "Echo v4"),
		zap.String("middleware_stack", "rate_limiting, security, compression, authentication"),
		zap.Bool("swagger_enabled", true),
		zap.String("swagger_endpoint", "/swagger/*"),
		zap.Bool("static_files_enabled", true),
		zap.String("static_path", "/static"),
		zap.String("management_interface", "/"),
	)

	// Log API endpoints configuration
	appLogger.Info("API Endpoints Configuration",
		zap.String("api_version", "v1"),
		zap.String("api_base_path", "/api/v1"),
		zap.Strings("content_endpoints", []string{
			"POST /api/v1/content/",
			"GET /api/v1/content/",
			"GET /api/v1/content/count",
			"GET /api/v1/content/{id}",
			"GET /api/v1/content/{id}/status",
			"DELETE /api/v1/content/{id}",
		}),
		zap.Strings("management_endpoints", []string{
			"POST /api/v1/sync",
			"POST /api/v1/backup",
			"POST /api/v1/cleanup",
			"POST /api/v1/gc",
			"GET /api/v1/metrics",
		}),
		zap.Strings("health_endpoints", []string{
			"GET /health",
			"GET /health/detailed",
		}),
	)

	if cfg.EnableProfiler {
		appLogger.Info("Debug Endpoints Configuration",
			zap.Bool("profiler_enabled", true),
			zap.String("profiler_path", "/debug/pprof/*"),
			zap.String("warning", "Profiler endpoints are enabled - disable in production"),
		)
	}

	// Display server startup information and available endpoints
	displayServerInfo(cfg)

	displayControlInstructions()

	// Configure and start server based on TLS configuration
	if cfg.EnableTLS {
		appLogger.Info("Starting HTTPS server with AutoTLS",
			zap.String("port", cfg.TLSPort),
			zap.Strings("hosts", cfg.TLSHosts),
			zap.Bool("https_only", cfg.EnableHTTPSOnly),
		)
		// Configure AutoTLS and start HTTPS server with proper shutdown handling
		tls.StartAutoTLSServerWithShutdown(router, cfg, appLogger, shutdownHandler, store)
	} else {
		appLogger.Info("Starting HTTP server",
			zap.String("host", cfg.Host),
			zap.String("port", cfg.Port),
			zap.String("full_address", fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)),
		)
		// Start regular HTTP server
		startServerWithShutdown(router, cfg, appLogger, shutdownHandler, store)
	}
}

// initializeStorage creates and configures the BadgerDB storage system with performance optimizations.
// It sets up the database, backup system, garbage collection, and health monitoring.
func initializeStorage(cfg *config.Config, appLogger *zap.Logger) (storage.Storage, error) {
	appLogger.Info("Initializing BadgerDB storage with performance and reliability enhancements...")

	badgerDir := filepath.Join(cfg.DataDir, "badger")
	backupDir := cfg.BackupDir

	badgerOpts := storage.BadgerOptions{
		DataDir:          badgerDir,
		BackupDir:        backupDir,
		BackupInterval:   cfg.BackupInterval,
		MaxBackups:       cfg.MaxBackups,
		EnableAdaptiveGC: cfg.EnableAdaptiveGC,
		LowTrafficHours:  []int{2, 3, 4, 5}, // Optimal GC hours: 2-5 AM for minimal user impact
		GCInterval:       cfg.GCInterval,
		GCThreshold:      cfg.GCThreshold, // Threshold for triggering aggressive cleanup

		// Performance optimization settings
		PerformanceMode:   cfg.PerformanceMode,   // Enables write batching and reduced sync frequency
		CacheSize:         cfg.CacheSize,         // In-memory cache size for frequently accessed content
		EnableCacheWarmup: cfg.EnableCacheWarmup, // Pre-loads frequently accessed data on startup

		// Write queue settings for sequential processing and consistency
		WriteQueueSize:         cfg.WriteQueueSize,         // Buffer size for pending write operations
		WriteQueueBatchSize:    cfg.WriteQueueBatchSize,    // Number of operations per batch
		WriteQueueBatchTimeout: cfg.WriteQueueBatchTimeout, // Maximum wait time before processing partial batch

		// Cache configuration for read performance
		CacheTTL: 5 * time.Minute, // Time-to-live for cached entries
	}

	// Initialize storage with emergency recovery if enabled
	badgerStore, err := storage.NewBadgerStorageWithRecovery(badgerOpts, cfg.EnableEmergencyRecovery)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize BadgerDB storage: %w", err)
	}

	// Log emergency recovery status
	if cfg.EnableEmergencyRecovery {
		appLogger.Info("Emergency recovery enabled - server will attempt to restore from emergency shutdown files")
	} else {
		appLogger.Info("Emergency recovery disabled - emergency shutdown files will be ignored")
	}

	// Start adaptive garbage collection loop for automatic cleanup
	badgerStore.StartGCLoop(cfg.GCInterval)

	// Start automated backup system with configured interval
	if err := badgerStore.StartBackups(); err != nil {
		appLogger.Warn("Failed to start automated backups", zap.Error(err))
	} else {
		appLogger.Info("Automated backup system started")
	}

	// Start continuous health monitoring for system status tracking
	badgerStore.StartHealthMonitoring()
	appLogger.Info("Health monitoring system started")

	// Log comprehensive storage initialization details
	appLogger.Info("BadgerDB Storage System Initialized",
		zap.String("badger_data_dir", badgerDir),
		zap.String("backup_dir", backupDir),
		zap.Duration("backup_interval", badgerOpts.BackupInterval),
		zap.Int("max_backups", badgerOpts.MaxBackups),
		zap.Bool("adaptive_gc_enabled", badgerOpts.EnableAdaptiveGC),
		zap.Ints("low_traffic_hours", badgerOpts.LowTrafficHours),
		zap.Duration("gc_interval", badgerOpts.GCInterval),
		zap.Float64("gc_threshold", badgerOpts.GCThreshold),
	)

	appLogger.Info("BadgerDB Performance Configuration",
		zap.Bool("performance_mode", badgerOpts.PerformanceMode),
		zap.Int64("cache_size_bytes", badgerOpts.CacheSize),
		zap.Float64("cache_size_mb", float64(badgerOpts.CacheSize)/(1024*1024)),
		zap.Bool("cache_warmup_enabled", badgerOpts.EnableCacheWarmup),
		zap.Duration("cache_ttl", badgerOpts.CacheTTL),
	)

	appLogger.Info("BadgerDB Write Queue Configuration",
		zap.Int("write_queue_size", badgerOpts.WriteQueueSize),
		zap.Int("write_queue_batch_size", badgerOpts.WriteQueueBatchSize),
		zap.Duration("write_queue_batch_timeout", badgerOpts.WriteQueueBatchTimeout),
		zap.String("write_strategy", "sequential processing for consistency"),
	)

	// Get initial storage health status
	healthStatus := badgerStore.GetHealthStatus()
	appLogger.Info("Initial Storage Health Status", zap.Any("health_metrics", healthStatus))

	appLogger.Info("Storage System Ready",
		zap.String("status", "fully_initialized"),
		zap.String("storage_type", "BadgerDB"),
		zap.String("features", "backup, gc, health_monitoring, performance_optimization"),
	)

	return badgerStore, nil
}


// setupRouter configures the comprehensive HTTP router with enterprise-grade middleware and API routes.
//
// This function creates and configures the Echo HTTP router with a carefully ordered
// middleware stack and complete API route definitions for optimal performance and security.
//
// ## Middleware Stack (Applied in Order):
// 1. **Shutdown Middleware** - CRITICAL: Immediately refuses requests during shutdown
// 2. **Rate Limiting** - Multi-layer throttling with Echo and custom rate limiters
// 3. **Security Headers** - CORS, security headers, IP allowlisting
// 4. **Compression** - Response compression based on Accept-Encoding
// 5. **Authentication** - API key validation for protected endpoints
// 6. **Request Logging** - Structured request/response logging
//
// ## Route Categories:
// - **Health Endpoints**: Public health checks and system status
// - **Static Assets**: Management interface files and resources
// - **API v1 Routes**: RESTful content management and system operations
// - **Documentation**: Swagger API documentation
// - **Debug Endpoints**: Performance profiler (conditionally enabled)
//
// ## Security Configuration:
// - **Public Routes**: Health checks, static files, documentation
// - **Protected Routes**: All API v1 endpoints (when authentication enabled)
// - **IP Restrictions**: Configurable IP allowlisting for enhanced security
// - **CORS Policy**: Configurable cross-origin resource sharing
//
// ## Performance Optimizations:
// - **Middleware Ordering**: Optimized for minimal overhead on high-traffic routes
// - **Static File Serving**: Efficient static asset delivery
// - **Compression**: Automatic response compression for bandwidth optimization
//
// ## API Endpoint Structure:
// - **Content Management**: CRUD operations with async processing
// - **System Management**: Backup, sync, garbage collection operations
// - **Health Monitoring**: Basic and detailed health checks
// - **Metrics**: System performance and operational metrics
//
// Returns configured Echo router ready for HTTP server attachment.
func setupRouter(cfg *config.Config, contentHandler *handlers.ContentHandler, healthHandler *handlers.HealthHandler, shutdownHandler *handlers.ShutdownHandler, appLogger *zap.Logger) *echo.Echo {
	e := echo.New()

	// CRITICAL: Apply shutdown middleware FIRST to immediately refuse requests during shutdown
	e.Use(shutdownHandler.Middleware())

	// Apply comprehensive middleware stack (rate limiting, security, compression, auth)
	custommiddleware.SetupMiddleware(e, cfg, appLogger)

	// Health check routes (publicly accessible, no authentication required)
	e.GET("/health", healthHandler.HealthCheck)
	e.GET("/health/detailed", healthHandler.DetailedHealthCheck)

	// Static file serving for management interface assets (publicly accessible)
	e.Static("/static", "static")

	// Management interface at root path (must be registered after static routes)
	e.GET("/", func(c echo.Context) error {
		return c.File("static/index.html")
	})

	// Swagger API documentation (publicly accessible for development)
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// Performance profiler endpoints (conditionally enabled for debugging)
	if cfg.EnableProfiler {
		e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
		appLogger.Info("Profiler endpoints enabled at /debug/pprof/")
	}

	// API v1 route group (subject to authentication if enabled)
	api := e.Group("/api/v1")

	// Content management routes
	content := api.Group("/content")
	content.POST("/", contentHandler.StoreContent)     // Create new content
	content.GET("/", contentHandler.ListContent)       // List content with pagination
	content.GET("/count", contentHandler.GetContentCount) // Get total content count
	content.GET("/:id", contentHandler.GetContent)     // Retrieve specific content by ID
	content.GET("/:id/status", contentHandler.GetContentStatus) // Check content storage status
	content.DELETE("/:id", contentHandler.DeleteContent) // Delete content by ID

	// System management and monitoring routes
	api.POST("/sync", healthHandler.TriggerSync)           // Manual synchronization trigger
	api.POST("/backup", healthHandler.CreateBackup)       // Manual backup creation
	api.POST("/cleanup", healthHandler.CleanupAccessTrackers) // Cleanup access tracking data
	api.POST("/gc", healthHandler.TriggerGC)              // Manual garbage collection
	api.GET("/metrics", healthHandler.GetMetrics)         // System metrics and statistics

	return e
}


// startServerWithShutdown manages server shutdown with emergency capabilities
func startServerWithShutdown(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger, shutdownHandler *handlers.ShutdownHandler, storage storage.Storage) {
	// Phase 1

	// Log server startup
	appLogger.Info("Starting HTTP server", zap.String("port", cfg.Port))

	// Create HTTP server with configured timeouts
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:      e,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Start HTTP server in background goroutine for non-blocking operation
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			appLogger.Fatal("HTTP server failed to start", zap.Error(err))
		}
	}()

	displaySuccessInfo()

	// Phase 2

	// Setup signal handling
	quit := make(chan os.Signal, 2) // Buffer for detecting double signals
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	var firstSignal os.Signal
	var firstSignalTime time.Time

	for {
		sig := <-quit
		now := time.Now()

		// IMMEDIATELY set shutdown state to refuse new requests
		shutdownHandler.InitiateShutdown()

		if firstSignal == nil {
			fmt.Println("=== CTRL+C received, wait for 3s ...")
		}

		// Determine shutdown type
		isEmergency := false

		switch sig {
		case syscall.SIGUSR1:
			// SIGUSR1 always triggers emergency shutdown
			isEmergency = true
			fmt.Println("=== SIGUSR1")

		case syscall.SIGINT, syscall.SIGTERM:
			if firstSignal == nil {
				// First signal
				firstSignal = sig
				firstSignalTime = now

				// Wait for potential second signal
				go func() {
					time.Sleep(3 * time.Second)
					// If we're still waiting, proceed with graceful shutdown
					select {
					case quit <- syscall.SIGTERM: // Send a delayed graceful signal
						fmt.Println("=== SIGTERM")
					default:
						fmt.Println("=== SIGTERM (not accepted)")
					}
				}()
				continue

			} else {
				// Second signal - check timing
				timeSinceFirst := now.Sub(firstSignalTime)
				if timeSinceFirst <= 3*time.Second {
					isEmergency = true
				}
			}
		}

		// Execute shutdown
		if isEmergency {
			performEmergencyServerShutdown(server, cfg, appLogger, storage)
		} else {
			performGracefulServerShutdown(server, cfg, appLogger)
		}
		return
	}
}

// performGracefulServerShutdown performs a graceful server shutdown
func performGracefulServerShutdown(server *http.Server, cfg *config.Config, appLogger *zap.Logger) {
	startTime := time.Now()

	// Create shutdown deadline to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Create a channel to track shutdown completion
	shutdownDone := make(chan error, 1)

	// Start shutdown in goroutine to monitor progress
	go func() {
		err := server.Shutdown(ctx)
		shutdownDone <- err
	}()

	// Monitor shutdown progress
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-shutdownDone:
			totalDuration := time.Since(startTime)
			if err != nil {
				appLogger.Error("HTTP server shutdown failed", zap.Error(err), zap.Duration("duration", totalDuration))
				fmt.Printf("HTTP server shutdown - FAILED (%v) [%v]\n", err, totalDuration)
			} else {
				appLogger.Info("HTTP server shutdown completed", zap.Duration("duration", totalDuration))
				fmt.Printf("HTTP server shutdown - SUCCESS [%v]\n", totalDuration)
			}
			return

		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := cfg.ShutdownTimeout - elapsed
			fmt.Printf("HTTP server shutdown - TIMEOUT [%v] | [%v]\n", elapsed, remaining)

		case <-ctx.Done():
			elapsed := time.Since(startTime)
			appLogger.Error("HTTP server shutdown timeout", zap.Duration("elapsed", elapsed))
			fmt.Printf("HTTP server shutdown - TIMEOUT (%v) [%v]\n", ctx.Err(), elapsed)
			return
		}
	}
}

// performEmergencyServerShutdown performs an emergency server shutdown
func performEmergencyServerShutdown(server *http.Server, cfg *config.Config, appLogger *zap.Logger, storage storage.Storage) {
	startTime := time.Now()

	// Step 1: Perform emergency storage shutdown first (data preservation)
	if storageInstance, ok := storage.(interface{ EmergencyShutdown() error }); ok {
		if err := storageInstance.EmergencyShutdown(); err != nil {
			appLogger.Error("Emergency storage shutdown failed", zap.Error(err))
			fmt.Printf("Emergency storage shutdown - FAILED (%v)\n", err)
		}
	}

	// Step 2: Force immediate server shutdown
	if err := server.Close(); err != nil {
		elapsed := time.Since(startTime)
		appLogger.Error("HTTP server emergency shutdown failed", zap.Error(err), zap.Duration("duration", elapsed))
		fmt.Printf("HTTP server emergency shutdown - FAILED (%v) [%v]\n", err, elapsed)
	} else {
		elapsed := time.Since(startTime)
		appLogger.Info("HTTP server emergency shutdown completed", zap.Duration("duration", elapsed))
		fmt.Printf("HTTP server emergency shutdown - SUCCESS [%v]\n", elapsed)
	}

	// Step 3: Exit immediately (abandon server)
	fmt.Println("🚨 EMERGENCY SHUTDOWN: Server abandoned")
	os.Exit(1)
}

// logComprehensiveStartupInfo logs detailed server configuration and system status on startup.
// This provides complete operational visibility for debugging, monitoring, and audit purposes.
func logComprehensiveStartupInfo(appLogger *zap.Logger, cfg *config.Config) {
	// Get system information
	hostname, _ := os.Hostname()
	pid := os.Getpid()
	wd, _ := os.Getwd()

	// Get memory statistics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// appLogger.Info("=== COMPREHENSIVE SERVER STARTUP INFORMATION ===")

	// System Environment Information
	appLogger.Info("System Environment",
		zap.String("go_version", runtime.Version()),
		zap.String("go_os", runtime.GOOS),
		zap.String("go_arch", runtime.GOARCH),
		zap.String("hostname", hostname),
		zap.Int("process_id", pid),
		zap.String("working_directory", wd),
		zap.Int("cpu_count", runtime.NumCPU()),
		zap.Int("max_procs", runtime.GOMAXPROCS(0)),
	)

	// Memory and Runtime Statistics
	appLogger.Info("Runtime Memory Statistics",
		zap.Uint64("heap_alloc_bytes", memStats.HeapAlloc),
		zap.Uint64("heap_sys_bytes", memStats.HeapSys),
		zap.Uint64("heap_idle_bytes", memStats.HeapIdle),
		zap.Uint64("heap_inuse_bytes", memStats.HeapInuse),
		zap.Uint64("stack_inuse_bytes", memStats.StackInuse),
		zap.Uint64("stack_sys_bytes", memStats.StackSys),
		zap.Uint32("num_gc", memStats.NumGC),
		zap.Uint64("total_alloc_bytes", memStats.TotalAlloc),
	)

	// HTTP Server Configuration
	appLogger.Info("HTTP Server Configuration",
		zap.String("host", cfg.Host),
		zap.String("port", cfg.Port),
		zap.Duration("read_timeout", cfg.ReadTimeout),
		zap.Duration("write_timeout", cfg.WriteTimeout),
		zap.Duration("request_timeout", cfg.RequestTimeout),
		zap.Duration("shutdown_timeout", cfg.ShutdownTimeout),
	)

	// TLS/HTTPS Configuration
	if cfg.EnableTLS {
		appLogger.Info("TLS/HTTPS Configuration",
			zap.Bool("tls_enabled", cfg.EnableTLS),
			zap.String("tls_port", cfg.TLSPort),
			zap.String("tls_cache_dir", cfg.TLSCacheDir),
			zap.Strings("tls_hosts", cfg.TLSHosts),
			zap.Bool("https_only", cfg.EnableHTTPSOnly),
		)
	} else {
		appLogger.Info("TLS/HTTPS Configuration",
			zap.Bool("tls_enabled", false),
			zap.String("note", "Server will run in HTTP-only mode"),
		)
	}

	// Storage System Configuration
	appLogger.Info("Storage System Configuration",
		zap.String("data_directory", cfg.DataDir),
		zap.String("backup_directory", cfg.BackupDir),
		zap.Duration("backup_interval", cfg.BackupInterval),
		zap.Int("max_backups", cfg.MaxBackups),
		zap.Duration("gc_interval", cfg.GCInterval),
		zap.Bool("adaptive_gc_enabled", cfg.EnableAdaptiveGC),
		zap.Float64("gc_threshold", cfg.GCThreshold),
	)

	// Performance and Optimization Settings
	appLogger.Info("Performance Configuration",
		zap.Bool("performance_mode", cfg.PerformanceMode),
		zap.Int64("cache_size_bytes", cfg.CacheSize),
		zap.Float64("cache_size_mb", float64(cfg.CacheSize)/(1024*1024)),
		zap.Bool("cache_warmup_enabled", cfg.EnableCacheWarmup),
		zap.Int64("max_content_size_bytes", cfg.MaxContentSize),
		zap.Float64("max_content_size_mb", float64(cfg.MaxContentSize)/(1024*1024)),
	)

	// Write Queue Configuration
	appLogger.Info("Write Queue Configuration",
		zap.Int("queue_size", cfg.WriteQueueSize),
		zap.Int("batch_size", cfg.WriteQueueBatchSize),
		zap.Duration("batch_timeout", cfg.WriteQueueBatchTimeout),
		zap.String("workers", "1 (single writer for sequential ordering)"),
		zap.String("note", "Sequential processing ensures data consistency"),
	)

	// Authentication and Security Configuration
	appLogger.Info("Authentication & Security Configuration",
		zap.Bool("auth_enabled", cfg.EnableAuth),
		zap.Bool("api_key_configured", cfg.APIKey != ""),
		zap.Strings("allowed_origins", cfg.AllowedOrigins),
		zap.Strings("allowed_ips", cfg.AllowedIPs),
		zap.Bool("ip_restrictions", len(cfg.AllowedIPs) > 0),
		zap.Strings("allowed_content_types", cfg.AllowedContentTypes),
	)

	// Rate Limiting Configuration
	appLogger.Info("Rate Limiting Configuration",
		zap.Int("throttle_limit", cfg.ThrottleLimit),
		zap.Int("throttle_backlog_limit", cfg.ThrottleBacklogLimit),
		zap.Duration("throttle_backlog_timeout", cfg.ThrottleBacklogTimeout),
		zap.Duration("api_rate_limit_period", cfg.APIRateLimitPeriod),
		zap.Int("api_rate_limit_burst", cfg.APIRateLimitBurst),
		zap.Float64("echo_rate_limit", cfg.EchoRateLimit),
		zap.Int("echo_burst_limit", cfg.EchoBurstLimit),
		zap.Duration("echo_rate_limit_expires_in", cfg.EchoRateLimitExpiresIn),
	)

	// Compression Configuration
	if cfg.EnableCompression {
		appLogger.Info("Compression Configuration",
			zap.Bool("compression_enabled", cfg.EnableCompression),
			zap.Int("compression_level", cfg.CompressionLevel),
			zap.Strings("compression_types", cfg.CompressionTypes),
		)
	} else {
		appLogger.Info("Compression Configuration",
			zap.Bool("compression_enabled", false),
			zap.String("note", "Response compression is disabled"),
		)
	}

	// Validation Limits Configuration
	appLogger.Info("Validation Limits Configuration",
		zap.Int("max_id_length", cfg.MaxIDLength),
		zap.Int("max_type_length", cfg.MaxTypeLength),
		zap.Int("max_pagination_limit", cfg.MaxPaginationLimit),
		zap.Int("max_access_limit", cfg.MaxAccessLimit),
	)

	// Logging Configuration
	appLogger.Info("Logging Configuration",
		zap.Bool("request_logging", cfg.EnableRequestLogging),
		zap.Bool("management_logging", cfg.EnableManagementLogging),
		zap.Bool("security_logging", cfg.EnableSecurityLogging),
		zap.Bool("critical_logging", cfg.EnableCriticalLogging),
		zap.Bool("crud_logging_suppressed", cfg.SuppressCRUDLogging),
		zap.Bool("error_logging", cfg.EnableErrorLogging),
		zap.Bool("warn_logging", cfg.EnableWarnLogging),
		zap.Bool("validation_logging", cfg.EnableValidationLogging),
	)

	// Debug and Development Configuration
	appLogger.Info("Debug & Development Configuration",
		zap.Bool("profiler_enabled", cfg.EnableProfiler),
		zap.String("profiler_endpoint", "/debug/pprof/*"),
		zap.Bool("profiler_accessible", cfg.EnableProfiler),
	)

	// Directory Status Verification
	directories := []string{cfg.DataDir, cfg.BackupDir, "./logs"}
	if cfg.EnableTLS {
		directories = append(directories, cfg.TLSCacheDir)
	}

	// Directory Status Verification
	for _, dir := range directories {
		if stat, err := os.Stat(dir); err == nil {
			appLogger.Info("Directory Status",
				zap.String("path", dir),
				zap.String("status", "exists"),
				zap.Bool("is_directory", stat.IsDir()),
				zap.String("permissions", stat.Mode().String()),
				zap.Time("modified", stat.ModTime()),
			)
		} else {
			appLogger.Warn("Directory Status",
				zap.String("path", dir),
				zap.String("status", "missing or inaccessible"),
				zap.Error(err),
			)
		}
	}

	// Security Headers Configuration
	if len(cfg.SecurityHeaders) > 0 {
		// Security Headers Configuration
		for key, value := range cfg.SecurityHeaders {
			appLogger.Info("Security Header",
				zap.String("header", key),
				zap.String("value", value),
			)
		}
	}

	// appLogger.Info("=== SERVER STARTUP CONFIGURATION COMPLETE ===",
	// 	zap.String("status", "All configuration details logged"),
	// 	zap.String("next_step", "Initializing storage system"),
	// )
}

// printStartupBanner displays the server identification banner.
func printStartupBanner() {
	fmt.Println("🚀 Starting Content Storage Server...")
}

// createDirectories creates all necessary directories for server operation.
// It ensures data, backup, log, and TLS certificate directories exist with proper permissions.
func createDirectories(cfg *config.Config) {
	fmt.Println("📁 Creating data directories...")

	directories := []string{
		cfg.DataDir,   // Main data storage directory
		cfg.BackupDir, // Backup storage directory
		"./logs",      // Application log directory
	}

	// Add TLS cache directory if TLS is enabled
	if cfg.EnableTLS {
		directories = append(directories, cfg.TLSCacheDir)
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Warning: Failed to create directory %s: %v\n", dir, err)
		}
	}
}

// displayServerInfo shows comprehensive server startup information and available endpoints.
// It provides users with all necessary URLs and endpoint documentation for immediate use.
func displayServerInfo(cfg *config.Config) {
	if cfg.EnableTLS {
		fmt.Printf("✅ Starting HTTPS server on port %s...\n", cfg.TLSPort)
		if len(cfg.TLSHosts) > 0 {
			// Show URLs for configured hosts
			for _, host := range cfg.TLSHosts {
				fmt.Printf("🎛️  Management Interface: https://%s/\n", host)
				fmt.Printf("📊 Health check: https://%s/health\n", host)
				fmt.Printf("📖 Swagger API: https://%s/swagger/index.html\n", host)
			}
		} else {
			fmt.Printf("🎛️  Management Interface: https://your-domain/\n")
			fmt.Printf("📊 Health check: https://your-domain/health\n")
			fmt.Printf("📖 Swagger API: https://your-domain/swagger/index.html\n")
		}
		if cfg.EnableHTTPSOnly {
			fmt.Println("🔒 HTTPS-only mode: HTTP traffic will be redirected to HTTPS")
		}
	} else {
		fmt.Printf("✅ Starting HTTP server on %s:%s...\n", cfg.Host, cfg.Port)
		fmt.Printf("🎛️  Management Interface: http://%s:%s/\n", cfg.Host, cfg.Port)
		fmt.Printf("📊 Health check available at: http://%s:%s/health\n", cfg.Host, cfg.Port)
		fmt.Printf("📖 Swagger API documentation: http://%s:%s/swagger/index.html\n", cfg.Host, cfg.Port)
	}
	fmt.Println("📚 API endpoints:")
	fmt.Println("   POST   /api/v1/content/     - Store new content (returns HTTP 202 - queued)")
	fmt.Println("   GET    /api/v1/content/     - List content (with pagination)")
	fmt.Println("   GET    /api/v1/content/{id} - Get specific content by ID")
	fmt.Println("   GET    /api/v1/content/{id}/status - Check content storage status")
	fmt.Println("   DELETE /api/v1/content/{id} - Delete content by ID")
	fmt.Println("   GET    /api/v1/content/count - Get total content count")
	fmt.Println("   GET    /health              - Basic health check")
	fmt.Println("   GET    /health/detailed     - Detailed health check with metrics")
	fmt.Println("   GET    /api/v1/metrics      - System metrics and statistics")
	fmt.Println("   POST   /api/v1/sync         - Trigger manual synchronization")
	fmt.Println("   POST   /api/v1/backup       - Create manual backup")
	fmt.Println("   POST   /api/v1/gc           - Trigger garbage collection")
	fmt.Println("   POST   /api/v1/cleanup      - Cleanup access tracking data")
	fmt.Println()
}

func displayControlInstructions() {
	fmt.Println("🛑 SERVER CONTROL INSTRUCTIONS")
	fmt.Println("• Single Ctrl+C or SIGTERM: Graceful shutdown (immediately refuses new requests, waits for pending operations)")
	fmt.Println("• Double Ctrl+C within 3 seconds: Emergency shutdown (immediately refuses new requests + immediate backup + stop)")
	fmt.Println("• SIGUSR1 signal: Emergency shutdown (immediately refuses new requests + immediate backup + stop)")
	fmt.Println("• All new requests return HTTP 503 immediately upon any shutdown signal")
	fmt.Println("• Emergency backups saved to: backups/emergency/")
	fmt.Println()
}


func displaySuccessInfo() {
	fmt.Println("🟢 Server started successfully.")
	fmt.Println()
}