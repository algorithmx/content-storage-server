package handlers

import (
	"net/http"
	"time"

	"content-storage-server/pkg/models"
	"content-storage-server/pkg/storage"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// HealthHandler handles health check and monitoring endpoints
type HealthHandler struct {
	storage   storage.Storage
	startTime time.Time
	logger    *zap.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(storage storage.Storage, appLogger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		storage:   storage,
		startTime: time.Now(),
		logger:    appLogger,
	}
}

// HealthCheck handles GET /health
// @Summary Basic health check with storage connectivity verification
// @Description Simple health check endpoint that verifies server responsiveness and basic storage connectivity for load balancers and monitoring systems
// @Description
// @Description ## Configuration Dependencies:
// @Description - **No authentication required** - Always accessible regardless of ENABLE_AUTH setting
// @Description - **No rate limiting applied** - Excluded from THROTTLE_LIMIT restrictions
// @Description - **Storage type independent** - Works with any storage backend
// @Description
// @Description ## Health Verification:
// @Description - Verifies server process is responding to HTTP requests
// @Description - Tests basic storage connectivity by performing a count operation
// @Description - Returns server uptime since startup for monitoring purposes
// @Description - Always returns HTTP 200 if server can respond (fail-safe design)
// @Description
// @Description ## Response Data:
// @Description - status: Always "healthy" (indicates server responsiveness)
// @Description - timestamp: Current server time in RFC3339 format
// @Description - uptime: Duration since server startup
// @Description - metrics.content_count: Total content items (verifies storage connectivity)
// @Tags Health & Monitoring
// @Accept json
// @Produce json
// @Success 200 {object} SwaggerHealthResponse "Server is healthy and storage is accessible"
// @Router /health [get]
func (h *HealthHandler) HealthCheck(c echo.Context) error {
	// Basic health check - just verify we can respond
	response := models.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(h.startTime),
	}

	// Try to get a simple metric from storage to verify it's working
	count := h.storage.Count()
	response.Metrics = map[string]interface{}{
		"content_count": count,
	}

	return c.JSON(http.StatusOK, response)
}

// DetailedHealthCheck handles GET /health/detailed
// @Summary Detailed health check with comprehensive system diagnostics
// @Description Comprehensive health check with detailed system metrics, storage diagnostics, and adaptive HTTP status codes
// @Description
// @Description ## Configuration Dependencies:
// @Description - **No authentication required** - Always accessible regardless of ENABLE_AUTH setting
// @Description - **No rate limiting applied** - Excluded from THROTTLE_LIMIT restrictions
// @Description - **Storage type dependent** - Returns different metrics based on storage backend
// @Description
// @Description ## BadgerDB-Specific Diagnostics:
// @Description - Database health metrics (size, corruption detection, performance)
// @Description - Backup system status (last backup time, backup count, backup health)
// @Description - Garbage collection statistics (frequency, duration, effectiveness)
// @Description - Write queue health (queue depth, processing rate, emergency status)
// @Description - Access manager statistics (tracker count, cleanup frequency)
// @Description
// @Description ## Adaptive HTTP Status Codes:
// @Description - **200 OK**: All systems healthy and performing optimally
// @Description - **206 Partial Content**: System degraded but functional (high queue depth, slow GC, etc.)
// @Description - **503 Service Unavailable**: System unhealthy (database corruption, backup failures, etc.)
// @Description
// @Description ## Fallback for Other Storage Types:
// @Description - Returns basic metrics (content count, storage type) for non-BadgerDB backends
// @Description - Always returns HTTP 200 for unknown storage types
// @Tags Health & Monitoring
// @Accept json
// @Produce json
// @Success 200 {object} SwaggerDetailedHealthResponse "System is healthy"
// @Success 206 {object} SwaggerDetailedHealthResponse "System is degraded"
// @Failure 503 {object} SwaggerDetailedHealthResponse "System is unhealthy"
// @Router /health/detailed [get]
func (h *HealthHandler) DetailedHealthCheck(c echo.Context) error {
	response := models.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(h.startTime),
	}

	// Get detailed metrics from storage if it's a BadgerStorage
	if badgerStorage, ok := h.storage.(*storage.BadgerStorage); ok {
		healthStatus := badgerStorage.GetHealthStatus()
		response.Metrics = healthStatus

		// Add queue metrics to detailed health response
		if queueMetrics := badgerStorage.GetQueueMetrics(); queueMetrics != nil {
			// Add queue metrics to the existing metrics map
			if response.Metrics == nil {
				response.Metrics = make(map[string]interface{})
			}
			response.Metrics["queue_metrics"] = map[string]interface{}{
				"queue_depth":      queueMetrics.QueueDepth,
				"total_queued":     queueMetrics.TotalQueued,
				"total_processed":  queueMetrics.TotalProcessed,
				"total_errors":     queueMetrics.TotalErrors,
			}
		}

		// Determine overall health status based on storage health
		databaseHealthMetrics := badgerStorage.GetDatabaseHealthMetrics()
		switch databaseHealthMetrics {
		case storage.HealthStatusUnhealthy:
			response.Status = "unhealthy"
		case storage.HealthStatusDegraded:
			response.Status = "degraded"
		default:
			response.Status = "healthy"
		}
	} else {
		// Fallback for other storage types
		response.Metrics = map[string]interface{}{
			"content_count": h.storage.Count(),
			"storage_type":  "unknown",
		}
	}

	// Set appropriate HTTP status based on health
	statusCode := http.StatusOK
	if response.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	} else if response.Status == "degraded" {
		statusCode = http.StatusPartialContent
	}

	return c.JSON(statusCode, response)
}

// GetMetrics handles GET /api/v1/metrics
// @Summary Get comprehensive system metrics with operational intelligence
// @Description Retrieve detailed system metrics including storage health, performance statistics, and operational intelligence for monitoring and alerting
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **Storage backend dependent** - Metrics vary based on storage implementation
// @Description - **Backup configuration** - Backup stats depend on BACKUP_INTERVAL and backup settings
// @Description
// @Description ## BadgerDB Metrics Categories:
// @Description - **Content Statistics**: Total count, database size, growth trends
// @Description - **Storage Health**: Database health status, corruption detection, disk usage
// @Description - **Backup System**: Last backup time, backup count, backup success rate, backup sizes
// @Description - **Garbage Collection**: Run frequency, duration statistics, space reclaimed, efficiency
// @Description - **Access Management**: Active tracker count, cleanup statistics, memory usage
// @Description - **System Health**: Overall health status (healthy/degraded/unhealthy)
// @Description
// @Description ## Health Status Determination:
// @Description - **healthy**: All systems operating within normal parameters
// @Description - **degraded**: Some systems showing performance issues but functional
// @Description - **unhealthy**: Critical systems failing or corrupted
// @Description
// @Description ## Use Cases:
// @Description - Monitoring dashboards and alerting systems
// @Description - Performance analysis and capacity planning
// @Description - Troubleshooting storage and performance issues
// @Description - Automated health checks and SLA monitoring
// @Tags Health & Monitoring
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Success 200 {object} SwaggerMetricsResponse "Metrics retrieved successfully"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/metrics [get]
func (h *HealthHandler) GetMetrics(c echo.Context) error {
	response := models.MetricsResponse{
		ContentCount: h.storage.Count(),
		HealthStatus: "healthy",
	}

	// Get detailed metrics from storage if it's a BadgerStorage
	if badgerStorage, ok := h.storage.(*storage.BadgerStorage); ok {
		// Get health metrics
		databaseHealthMetrics := badgerStorage.GetDatabaseHealthMetrics()
		switch databaseHealthMetrics {
		case storage.HealthStatusUnhealthy:
			response.HealthStatus = "unhealthy"
		case storage.HealthStatusDegraded:
			response.HealthStatus = "degraded"
		default:
			response.HealthStatus = "healthy"
		}

		// Get backup stats
		if backupStats, err := badgerStorage.GetBackupStats(); err == nil {
			response.BackupStats = backupStats
			if lastBackupTime, ok := backupStats["last_backup_time"].(time.Time); ok {
				response.LastBackup = &lastBackupTime
			}
			if backupCount, ok := backupStats["backup_count"].(int); ok {
				response.BackupCount = backupCount
			}
		}



		// Get GC stats
		if gcStats := badgerStorage.GetGCStats(); gcStats != nil {
			response.GCStats = gcStats
		}

		// Get actual database size from BadgerDB
		if dbSize, err := badgerStorage.GetDatabaseSize(); err == nil {
			response.DatabaseSize = dbSize
		} else {
			response.DatabaseSize = 0
		}

		// Get access manager stats for memory leak monitoring
		if accessManager := badgerStorage.GetAccessManager(); accessManager != nil {
			response.AccessManagerStats = accessManager.GetStats()
		}

		// Get queue metrics for write queue monitoring
		if queueMetrics := badgerStorage.GetQueueMetrics(); queueMetrics != nil {
			response.QueueMetrics = map[string]interface{}{
				"queue_depth":      queueMetrics.QueueDepth,
				"total_queued":     queueMetrics.TotalQueued,
				"total_processed":  queueMetrics.TotalProcessed,
				"total_errors":     queueMetrics.TotalErrors,
			}
		}
	}

	return c.JSON(http.StatusOK, response)
}

// TriggerSync handles POST /api/v1/sync
// @Summary Trigger manual database synchronization (stub implementation)
// @Description Manually trigger a database synchronization operation with comprehensive logging and monitoring
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **Storage backend dependent** - Sync behavior varies by storage implementation
// @Description
// @Description ## Current Implementation:
// @Description - **Stub implementation**: No actual synchronization logic is performed
// @Description - Comprehensive request logging with client IP, request ID, and user agent
// @Description - Duration tracking for monitoring and performance analysis
// @Description - Always returns success for compatibility with monitoring systems
// @Description
// @Description ## Future Enhancement:
// @Description - Intended for database synchronization operations
// @Description - May improve performance after heavy write operations
// @Description - Could include write queue flushing and database consistency checks
// @Tags Management Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Success 200 {object} SwaggerSuccessResponse "Sync completed successfully"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Sync operation failed"
// @Router /api/v1/sync [post]
func (h *HealthHandler) TriggerSync(c echo.Context) error {
	// Log sync operation initiation with client details
	clientIP := c.RealIP()
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	h.logger.Info("Manual sync operation triggered",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.String("user_agent", c.Request().UserAgent()),
		zap.String("endpoint", "/api/v1/sync"))

	startTime := time.Now()

	duration := time.Since(startTime)
	h.logger.Info("Manual sync operation completed successfully",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.Duration("duration", duration))

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Sync completed successfully",
		Data:    nil,
	})
}

// TriggerGC handles POST /api/v1/gc - Manual garbage collection trigger
// @Summary Trigger manual garbage collection
// @Description Manually trigger database garbage collection to reclaim disk space and optimize performance
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **Storage backend dependent** - Only supported for BadgerDB storage
// @Description
// @Description ## Behavior:
// @Description - Forces immediate garbage collection on the database
// @Description - Reclaims disk space from deleted or expired content
// @Description - May temporarily impact performance during operation
// @Description - Operation duration varies based on database size
// @Tags Management Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Success 200 {object} SwaggerGCResponse "Garbage collection completed successfully"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Garbage collection failed"
// @Router /api/v1/gc [post]
func (h *HealthHandler) TriggerGC(c echo.Context) error {
	// Log GC operation initiation
	clientIP := c.RealIP()
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	h.logger.Info("Manual GC operation triggered",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.String("endpoint", "/api/v1/gc"))

	startTime := time.Now()

	// Trigger GC if storage supports it
	if badgerStorage, ok := h.storage.(*storage.BadgerStorage); ok {
		if err := badgerStorage.RunGC(); err != nil {
			h.logger.Error("Failed to run GC", zap.Error(err))
			return c.JSON(http.StatusInternalServerError, models.StorageResponse{
				Success: false,
				Message: "Failed to run garbage collection",
				Data:    map[string]string{"error": err.Error()},
			})
		}
	}

	duration := time.Since(startTime)
	h.logger.Info("Manual GC operation completed",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.Duration("duration", duration))

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Garbage collection completed successfully",
		Data:    map[string]interface{}{"duration": duration.String()},
	})
}

// CreateBackup handles POST /api/v1/backup
// @Summary Create manual database backup with comprehensive monitoring
// @Description Create a manual backup of the database with optional configuration, comprehensive logging, and detailed response information
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **BACKUP_DIR**: Determines where backups are stored (from server configuration)
// @Description - **Storage backend dependent** - Only supported for BadgerDB storage
// @Description
// @Description ## Request Parameters:
// @Description - **backup_name**: Optional custom backup name (defaults to timestamp-based naming)
// @Description - **immediate**: Optional flag to force immediate backup (defaults to true)
// @Description - Invalid JSON in request body defaults to immediate backup with auto-generated name
// @Description
// @Description ## Backup Process:
// @Description - Creates immediate backup regardless of BACKUP_INTERVAL schedule
// @Description - Backup includes all current data, metadata, and access tracking information
// @Description - Returns detailed backup information including path, size, and creation time
// @Description - Operation duration varies with database size (logged for monitoring)
// @Description - Comprehensive request logging with client IP, request ID, and user agent
// @Description
// @Description ## Response Information:
// @Description - Backup path and file size from storage backend
// @Description - Creation timestamp and operation duration
// @Description - Backup statistics for monitoring and verification
// @Tags Management Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Param backup body SwaggerBackupRequest false "Backup configuration (optional)"
// @Success 200 {object} SwaggerBackupResponse "Backup created successfully"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Backup operation failed"
// @Failure 501 {object} SwaggerErrorResponse "Backup not supported for storage type"
// @Router /api/v1/backup [post]
func (h *HealthHandler) CreateBackup(c echo.Context) error {
	// Log backup operation initiation with client details
	clientIP := c.RealIP()
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	h.logger.Info("Manual backup operation triggered",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.String("user_agent", c.Request().UserAgent()),
		zap.String("endpoint", "/api/v1/backup"))

	var req models.BackupRequest
	if err := c.Bind(&req); err != nil {
		// If decode fails, just use default values
		req = models.BackupRequest{Immediate: true}
		h.logger.Debug("Using default backup request parameters",
			zap.String("client_ip", clientIP),
			zap.String("request_id", requestID),
			zap.Bool("immediate", req.Immediate))
	} else {
		h.logger.Debug("Backup request parameters",
			zap.String("client_ip", clientIP),
			zap.String("request_id", requestID),
			zap.String("backup_name", req.BackupName),
			zap.Bool("immediate", req.Immediate))
	}

	// Check if storage supports backup operations
	badgerStorage, ok := h.storage.(*storage.BadgerStorage)
	if !ok {
		h.logger.Warn("Backup operation not supported for storage type",
			zap.String("client_ip", clientIP),
			zap.String("request_id", requestID),
			zap.String("storage_type", "non-badger"))
		return c.JSON(http.StatusNotImplemented, models.StorageResponse{
			Success: false,
			Message: "Backup not supported for this storage type",
			Data:    nil,
		})
	}

	startTime := time.Now()

	if err := badgerStorage.CreateBackup(); err != nil {
		duration := time.Since(startTime)
		h.logger.Error("Manual backup operation failed",
			zap.Error(err),
			zap.String("client_ip", clientIP),
			zap.String("request_id", requestID),
			zap.Duration("duration", duration))
		return c.JSON(http.StatusInternalServerError, models.StorageResponse{
			Success: false,
			Message: "Backup operation failed",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	duration := time.Since(startTime)

	// Get backup stats to return information about the created backup
	backupStats, _ := badgerStorage.GetBackupStats()

	h.logger.Info("Manual backup operation completed successfully",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.Duration("duration", duration),
		zap.Any("backup_stats", backupStats))

	response := models.BackupResponse{
		Success:   true,
		Message:   "Backup created successfully",
		CreatedAt: time.Now(),
	}

	if backupStats != nil {
		if lastBackupPath, ok := backupStats["last_backup_path"].(string); ok {
			response.BackupPath = lastBackupPath
		}
		if lastBackupSize, ok := backupStats["last_backup_size"].(int64); ok {
			response.BackupSize = lastBackupSize
		}
	}

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Backup created successfully",
		Data:    response,
	})
}

// CleanupAccessTrackers manually triggers access tracker cleanup
// @Summary Cleanup access trackers
// @Description Manually trigger cleanup of stale access trackers to prevent memory leaks
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **Storage backend dependent** - Only supported for BadgerDB storage
// @Description
// @Description ## Behavior:
// @Description - Removes stale access tracking data for non-existent content
// @Description - Prevents memory leaks from accumulating access counters
// @Description - Returns count of removed trackers and operation duration
// @Description - Safe to run during normal operations
// @Tags Management Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Success 200 {object} SwaggerCleanupResponse "Cleanup completed successfully"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/cleanup [post]
func (h *HealthHandler) CleanupAccessTrackers(c echo.Context) error {
	// Get client information for logging
	clientIP := c.RealIP()
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)

	h.logger.Info("Manual access tracker cleanup triggered",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.String("user_agent", c.Request().UserAgent()),
		zap.String("endpoint", "/api/v1/cleanup"))

	startTime := time.Now()

	// Perform cleanup if storage supports it
	var removedCount int
	if badgerStorage, ok := h.storage.(*storage.BadgerStorage); ok {
		if contentIDs, err := badgerStorage.GetAllContentIDs(); err == nil {
			removedCount = badgerStorage.GetAccessManager().CleanupExpiredTrackers(contentIDs)
		} else {
			h.logger.Error("Failed to get content IDs for cleanup", zap.Error(err))
			return c.JSON(http.StatusInternalServerError, models.StorageResponse{
				Success: false,
				Message: "Failed to perform cleanup",
				Data:    map[string]string{"error": err.Error()},
			})
		}
	}

	duration := time.Since(startTime)
	h.logger.Info("Manual access tracker cleanup completed",
		zap.String("client_ip", clientIP),
		zap.String("request_id", requestID),
		zap.Duration("duration", duration),
		zap.Int("removed_count", removedCount))

	response := map[string]interface{}{
		"removed_trackers": removedCount,
		"duration_ms":      duration.Milliseconds(),
	}

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Access tracker cleanup completed successfully",
		Data:    response,
	})
}
