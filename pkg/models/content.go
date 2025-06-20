package models

import (
	"sync/atomic"
	"time"
)

// Content represents a piece of stored content with access controls (now read-only)
type Content struct {
	ID          string     `json:"id"`
	Data        string     `json:"data"`
	Type        string     `json:"type"`
	Tag         string     `json:"tag,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	AccessLimit int        `json:"access_limit,omitempty"`
	// AccessCount removed - now tracked separately in AccessTracker
}

// AccessTracker tracks access count for a specific content item
type AccessTracker struct {
	ID          string `json:"id"`
	AccessCount int64  `json:"access_count"`
}

// IncrementAccessCount atomically increases the access count by 1 and returns the new value
func (at *AccessTracker) IncrementAccessCount() int64 {
	return atomic.AddInt64(&at.AccessCount, 1)
}

// GetAccessCount atomically gets the current access count
func (at *AccessTracker) GetAccessCount() int64 {
	return atomic.LoadInt64(&at.AccessCount)
}

// SetAccessCount atomically sets the access count to a specific value
func (at *AccessTracker) SetAccessCount(count int64) {
	atomic.StoreInt64(&at.AccessCount, count)
}

// IsExpired checks if the content has expired based on time or access count
// Now requires access count to be passed as parameter
func (c *Content) IsExpired(accessCount int64) bool {
	// Check time-based expiration
	if c.ExpiresAt != nil && time.Now().After(*c.ExpiresAt) {
		return true
	}

	// Check access count-based expiration
	// Content expires when access count exceeds the limit (not when it equals)
	if c.AccessLimit > 0 && accessCount > int64(c.AccessLimit) {
		return true
	}

	return false
}

// ContentWithAccess represents content with its current access count for API responses
type ContentWithAccess struct {
	*Content
	AccessCount int64 `json:"access_count"`
}

// StorageRequest represents a request to store content
type StorageRequest struct {
	ID          string     `json:"id" validate:"required,max=255,alphanum_underscore_dash_dot"`
	Data        string     `json:"data" validate:"required"`
	Type        string     `json:"type" validate:"required,max=100"`
	Tag         string     `json:"tag,omitempty" validate:"omitempty,max=100"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	AccessLimit int        `json:"access_limit,omitempty" validate:"omitempty,min=0,max=1000000"`
}

// StorageResponse represents a response from storage operations
type StorageResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// HealthResponse represents the health status of the storage server
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Uptime    time.Duration          `json:"uptime"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
}

// MetricsResponse represents storage metrics
type MetricsResponse struct {
	ContentCount       int                    `json:"content_count"`
	DatabaseSize       int64                  `json:"database_size_bytes"`
	LastBackup         *time.Time             `json:"last_backup,omitempty"`
	BackupCount        int                    `json:"backup_count"`
	HealthStatus       string                 `json:"health_status"`
	GCStats            map[string]interface{} `json:"gc_stats,omitempty"`
	BackupStats        map[string]interface{} `json:"backup_stats,omitempty"`
	AccessManagerStats map[string]interface{} `json:"access_manager_stats,omitempty"`
	QueueMetrics       map[string]interface{} `json:"queue_metrics,omitempty"`
}

// BackupRequest represents a backup operation request
type BackupRequest struct {
	BackupName string `json:"backup_name,omitempty"`
	Immediate  bool   `json:"immediate,omitempty"`
}

// BackupResponse represents a backup operation response
type BackupResponse struct {
	Success    bool      `json:"success"`
	Message    string    `json:"message,omitempty"`
	BackupPath string    `json:"backup_path,omitempty"`
	BackupSize int64     `json:"backup_size_bytes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
