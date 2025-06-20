package handlers

import (
	"time"

	"content-storage-server/pkg/models"
)

// SwaggerStorageRequest represents the request body for storing content
// @Description Request body for storing content with configuration-dependent validation
type SwaggerStorageRequest struct {
	// Content ID - must be unique, alphanumeric with dots, hyphens, underscores (max 255 chars)
	ID string `json:"id" example:"user-123-document" validate:"required" description:"Unique content identifier"`

	// Content data - any JSON type, size limited by MAX_CONTENT_SIZE configuration (default: 10MB)
	Data string `json:"data" example:"Hello, World!" description:"Content data of any JSON type"`

	// Content type - validated against ALLOWED_CONTENT_TYPES configuration (default: application/json,text/plain)
	Type string `json:"type" example:"text/plain" validate:"required" description:"MIME type of the content"`

	// Optional tag for categorization and filtering
	Tag string `json:"tag,omitempty" example:"user-documents" description:"Optional tag for content categorization"`

	// Optional expiration time - content becomes inaccessible after this time
	ExpiresAt *time.Time `json:"expires_at,omitempty" example:"2024-12-31T23:59:59Z" description:"Optional expiration timestamp (RFC3339 format)"`

	// Optional access limit - content becomes inaccessible after this many retrievals (max: 1,000,000)
	AccessLimit int `json:"access_limit,omitempty" example:"10" description:"Optional limit on number of times content can be accessed"`
}

// SwaggerContent represents stored content with metadata
// @Description Stored content with metadata and access tracking
type SwaggerContent struct {
	// Content ID
	ID string `json:"id" example:"user-123-document"`

	// Content data
	Data string `json:"data" example:"Hello, World!"`

	// Content type
	Type string `json:"type" example:"text/plain"`

	// Content tag
	Tag string `json:"tag,omitempty" example:"user-documents"`

	// Creation timestamp
	CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`

	// Expiration timestamp
	ExpiresAt *time.Time `json:"expires_at,omitempty" example:"2024-12-31T23:59:59Z"`

	// Access limit
	AccessLimit int `json:"access_limit,omitempty" example:"10"`

	// Current access count
	AccessCount int `json:"access_count" example:"3"`
}

// SwaggerSuccessResponse represents a successful API response
// @Description Standard success response format
type SwaggerSuccessResponse struct {
	// Success indicator
	Success bool `json:"success" example:"true"`

	// Success message
	Message string `json:"message" example:"Operation completed successfully"`

	// Response data (varies by endpoint)
	Data map[string]interface{} `json:"data"`
}

// SwaggerErrorResponse represents an error API response
// @Description Standard error response format
type SwaggerErrorResponse struct {
	// Success indicator (always false for errors)
	Success bool `json:"success" example:"false"`

	// Error message
	Message string `json:"message" example:"Validation failed"`

	// Error details
	Data struct {
		Error string `json:"error" example:"Content size exceeds maximum allowed size"`
	} `json:"data"`
}

// SwaggerContentListResponse represents the response for listing content
// @Description Response for content listing with pagination and filtering
type SwaggerContentListResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Content listed successfully"`
	Data    struct {
		// Array of content items
		Contents []SwaggerContent `json:"contents"`

		// Total number of items matching the filter
		Total int `json:"total" example:"150"`

		// Number of items per page (limited by configuration, max: 1000)
		Limit int `json:"limit" example:"50"`

		// Number of items skipped
		Offset int `json:"offset" example:"0"`

		// Applied filters
		Filters map[string]string `json:"filters" example:"{\"type\":\"text/plain\",\"include_expired\":\"false\"}"`
	} `json:"data"`
}

// SwaggerContentStatusResponse represents content status check response
// @Description Content storage status response
type SwaggerContentStatusResponse struct {
	// Operation success status
	Success bool `json:"success" example:"true"`

	// Response message
	Message string `json:"message" example:"Content status retrieved successfully"`

	// Status data
	Data struct {
		// Content ID
		ID string `json:"id" example:"user-123-document"`

		// Storage status: queued, stored, or not_found
		Status string `json:"status" example:"stored" enum:"queued,stored,not_found"`
	} `json:"data"`
}

// SwaggerHealthResponse represents health check response
// @Description Health check response with system status
type SwaggerHealthResponse struct {
	// System status
	Status string `json:"status" example:"healthy"`

	// Response timestamp
	Timestamp time.Time `json:"timestamp" example:"2024-01-01T00:00:00Z"`

	// Server uptime
	Uptime string `json:"uptime" example:"1h30m45s"`

	// Basic metrics
	Metrics struct {
		ContentCount int `json:"content_count" example:"42"`
	} `json:"metrics"`
}

// SwaggerDetailedHealthResponse represents detailed health check response
// @Description Detailed health check with comprehensive system metrics
type SwaggerDetailedHealthResponse struct {
	// System status
	Status string `json:"status" example:"healthy"`

	// Response timestamp
	Timestamp time.Time `json:"timestamp" example:"2024-01-01T00:00:00Z"`

	// Server uptime
	Uptime string `json:"uptime" example:"1h30m45s"`

	// Comprehensive metrics
	Metrics struct {
		ContentCount   int       `json:"content_count" example:"42"`
		DatabaseStatus string    `json:"database_status" example:"healthy"`
		BackupStatus   string    `json:"backup_status" example:"healthy"`
		GCStatus       string    `json:"gc_status" example:"healthy"`
		LastBackup     time.Time `json:"last_backup" example:"2024-01-01T00:00:00Z"`
		MemoryUsage    int64     `json:"memory_usage" example:"1024000"`
		DiskUsage      int64     `json:"disk_usage" example:"5120000"`
	} `json:"metrics"`
}

// SwaggerMetricsResponse represents system metrics response
// @Description Comprehensive system metrics and statistics
type SwaggerMetricsResponse struct {
	ContentCount int       `json:"content_count" example:"42"`
	DatabaseSize int64     `json:"database_size_bytes" example:"5120000"`
	HealthStatus string    `json:"health_status" example:"healthy"`
	LastBackup   time.Time `json:"last_backup" example:"2024-01-01T00:00:00Z"`
	BackupCount  int       `json:"backup_count" example:"7"`
	GCStats       struct {
		LastRun       time.Time `json:"last_run" example:"2024-01-01T00:00:00Z"`
		RunsCount     int       `json:"runs_count" example:"10"`
		AvgDurationMs int       `json:"avg_duration_ms" example:"150"`
	} `json:"gc_stats"`
	QueueMetrics struct {
		QueueDepth     int64 `json:"queue_depth" example:"5"`
		TotalQueued    int64 `json:"total_queued" example:"1250"`
		TotalProcessed int64 `json:"total_processed" example:"1245"`
		TotalErrors    int64 `json:"total_errors" example:"0"`
	} `json:"queue_metrics"`

	BackupStats struct {
		TotalBackups   int   `json:"total_backups" example:"7"`
		LastBackupSize int64 `json:"last_backup_size" example:"1024000"`
	} `json:"backup_stats"`
}

// SwaggerBackupRequest represents backup creation request
// @Description Request body for creating manual backups
type SwaggerBackupRequest struct {
	// Optional backup name
	BackupName string `json:"backup_name,omitempty" example:"manual-backup-2024"`

	// Whether to create backup immediately
	Immediate bool `json:"immediate,omitempty" example:"true"`
}

// SwaggerBackupResponse represents backup creation response
// @Description Response for backup creation with details
type SwaggerBackupResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Backup created successfully"`
	Data    struct {
		Success    bool      `json:"success" example:"true"`
		Message    string    `json:"message" example:"Backup created successfully"`
		BackupPath string    `json:"backup_path" example:"/path/to/backup"`
		BackupSize int64     `json:"backup_size_bytes" example:"1024000"`
		CreatedAt  time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
	} `json:"data"`
}

// SwaggerCountResponse represents content count response
// @Description Response for content count endpoint
type SwaggerCountResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Content count retrieved successfully"`
	Data    struct {
		Count int `json:"count" example:"42"`
	} `json:"data"`
}

// SwaggerGCResponse represents garbage collection operation response
// @Description Response for manual garbage collection operations
type SwaggerGCResponse struct {
	// Operation success status
	Success bool `json:"success" example:"true"`

	// Response message
	Message string `json:"message" example:"Garbage collection completed successfully"`

	// GC operation data
	Data struct {
		// Duration of the GC operation
		Duration string `json:"duration" example:"150ms"`
	} `json:"data"`
}

// SwaggerCleanupResponse represents access tracker cleanup operation response
// @Description Response for access tracker cleanup operations
type SwaggerCleanupResponse struct {
	// Operation success status
	Success bool `json:"success" example:"true"`

	// Response message
	Message string `json:"message" example:"Access tracker cleanup completed successfully"`

	// Cleanup operation data
	Data struct {
		// Number of removed trackers
		RemovedTrackers int `json:"removed_trackers" example:"15"`

		// Duration of the cleanup operation in milliseconds
		DurationMs int64 `json:"duration_ms" example:"25"`
	} `json:"data"`
}

// Convert models to swagger models for documentation
func toSwaggerContent(content *models.Content) SwaggerContent {
	// Data is already a string in models.Content, so no conversion needed
	return SwaggerContent{
		ID:          content.ID,
		Data:        content.Data,
		Type:        content.Type,
		Tag:         content.Tag,
		CreatedAt:   content.CreatedAt,
		ExpiresAt:   content.ExpiresAt,
		AccessLimit: content.AccessLimit,
		AccessCount: 0, // Default to 0 for read-only content
	}
}

// Convert ContentWithAccess to swagger models for documentation
func toSwaggerContentWithAccess(content *models.ContentWithAccess) SwaggerContent {
	return SwaggerContent{
		ID:          content.Content.ID,
		Data:        content.Content.Data,
		Type:        content.Content.Type,
		Tag:         content.Content.Tag,
		CreatedAt:   content.Content.CreatedAt,
		ExpiresAt:   content.Content.ExpiresAt,
		AccessLimit: content.Content.AccessLimit,
		AccessCount: int(content.AccessCount),
	}
}
