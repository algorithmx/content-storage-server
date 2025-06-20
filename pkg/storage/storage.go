package storage

import (
	"content-storage-server/pkg/models"
	"errors"
	"time"
)

var (
	// ErrContentNotFound is returned when content with the given ID is not found
	ErrContentNotFound = errors.New("content not found")

	// ErrContentExpired is returned when content has expired
	ErrContentExpired = errors.New("content has expired")

	// ErrSyncFailed is returned when content synchronization fails
	ErrSyncFailed = errors.New("content synchronization failed")

	// ErrStorageNotReady is returned when storage is not ready for operations
	ErrStorageNotReady = errors.New("storage not ready")

	// ErrShutdownTimeout is returned when graceful shutdown times out
	ErrShutdownTimeout = errors.New("graceful shutdown timeout exceeded")

	// ErrComponentStillRunning is returned when a component fails to stop during shutdown
	ErrComponentStillRunning = errors.New("component still running after shutdown")
)

// ContentFilter defines filtering options for content listing
type ContentFilter struct {
	// ContentType filters by MIME type (e.g., "text/plain", "application/json")
	ContentType string

	// Tag filters by content tag/classifier
	Tag string

	// CreatedAfter filters content created after this time
	CreatedAfter *time.Time

	// CreatedBefore filters content created before this time
	CreatedBefore *time.Time

	// IncludeExpired whether to include expired content (default: false)
	IncludeExpired bool
}

// Storage defines the interface for content storage
type Storage interface {

	// Count returns the number of content items in storage
	Count() int

	// Store saves content to storage
	Store(content *models.Content) error

	// Get retrieves content by ID and increments access count
	// Returns ContentWithAccess that includes current access count
	Get(id string) (*models.ContentWithAccess, error)

	// Delete removes content from storage
	Delete(id string) error

	// Read-only operations (without access count increment)
	GetReadOnly(id string) (*models.Content, error)

	// Access management methods
	GetAccessCount(id string) int64
	SetAccessCount(id string, count int64)
	RemoveAccessTracking(id string)

	// List retrieves content items with pagination
	List(limit, offset int) ([]*models.Content, error)

	// ListWithFilter retrieves content items with pagination and filtering
	ListWithFilter(limit, offset int, filter *ContentFilter) ([]*models.Content, error)

	// CountWithFilter returns the number of content items matching the filter
	CountWithFilter(filter *ContentFilter) int

	// RunGC runs a garbage collection cycle
	RunGC() error

	// StartGCLoop starts the garbage collection loop with specified interval
	StartGCLoop(interval time.Duration)

	// StopGCLoop stops the garbage collection loop
	StopGCLoop()

	// Close closes the storage and releases resources with default timeout
	Close() error

	// CloseWithTimeout closes the storage with a specified timeout for graceful shutdown
	// This allows for controlled shutdown of all components with proper cleanup
	CloseWithTimeout(timeout time.Duration) error

	// Restart restarts all components after a shutdown
	// Returns error if components are still running or restart fails
	Restart() error

	// VerifyCleanShutdown verifies that all components have been properly shut down
	// Returns a map of component names to their shutdown status (true = stopped)
	VerifyCleanShutdown() map[string]bool

	// IsFullyShutdown returns true if all components have been properly shut down
	IsFullyShutdown() bool

	// WaitForContentWrite waits for a specific content ID to complete writing
	// Returns nil if write completes successfully, error if timeout or write fails
	WaitForContentWrite(contentID string, timeout time.Duration) error

	// FlushQueue triggers an explicit flush of the write queue, pausing the worker temporarily.
	FlushQueue() error

}
