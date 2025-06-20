package storage

import (
	"content-storage-server/pkg/models"
	"sync"
	"time"
)

// AccessManager manages access counts for content items in memory
// This provides fast access tracking without modifying the read-only Content structs
type AccessManager struct {
	// accessTrackers stores access count data by content ID
	accessTrackers sync.Map // map[string]*models.AccessTracker

	// mutex protects operations that need consistency across multiple trackers
	mutex sync.RWMutex

	// Cleanup tracking for memory leak prevention
	lastCleanupTime time.Time
	cleanupInterval time.Duration
}

// NewAccessManager creates a new access manager
func NewAccessManager() *AccessManager {
	return &AccessManager{
		lastCleanupTime: time.Now(),
		cleanupInterval: 30 * time.Minute, // Cleanup every 30 minutes
	}
}

// IncrementAccess atomically increments the access count for a content ID
// Returns the new access count value
func (am *AccessManager) IncrementAccess(contentID string) int64 {
	// Load or create access tracker
	tracker := am.getOrCreateTracker(contentID)
	return tracker.IncrementAccessCount()
}

// GetAccessCount returns the current access count for a content ID
// Returns 0 if the content has never been accessed
func (am *AccessManager) GetAccessCount(contentID string) int64 {
	if value, exists := am.accessTrackers.Load(contentID); exists {
		tracker := value.(*models.AccessTracker)
		return tracker.GetAccessCount()
	}
	return 0
}

// SetAccessCount sets the access count for a content ID to a specific value
// This is useful for restoring access counts from persistent storage
func (am *AccessManager) SetAccessCount(contentID string, count int64) {
	tracker := am.getOrCreateTracker(contentID)
	tracker.SetAccessCount(count)
}

// RemoveAccess removes access tracking for a content ID
// This should be called when content is deleted
func (am *AccessManager) RemoveAccess(contentID string) {
	am.accessTrackers.Delete(contentID)
}

// GetAllAccessCounts returns a map of all content IDs to their access counts
// This is useful for persistence or debugging
func (am *AccessManager) GetAllAccessCounts() map[string]int64 {
	result := make(map[string]int64)

	am.accessTrackers.Range(func(key, value interface{}) bool {
		contentID := key.(string)
		tracker := value.(*models.AccessTracker)
		result[contentID] = tracker.GetAccessCount()
		return true
	})

	return result
}

// GetTrackedContentIDs returns a slice of all content IDs being tracked
func (am *AccessManager) GetTrackedContentIDs() []string {
	var ids []string

	am.accessTrackers.Range(func(key, value interface{}) bool {
		ids = append(ids, key.(string))
		return true
	})

	return ids
}

// Clear removes all access tracking data
// This is useful for testing or cleanup
func (am *AccessManager) Clear() {
	am.accessTrackers.Range(func(key, value interface{}) bool {
		am.accessTrackers.Delete(key)
		return true
	})
}

// GetStats returns statistics about the access manager
func (am *AccessManager) GetStats() map[string]interface{} {
	trackedCount := 0
	totalAccesses := int64(0)

	am.accessTrackers.Range(func(key, value interface{}) bool {
		trackedCount++
		tracker := value.(*models.AccessTracker)
		totalAccesses += tracker.GetAccessCount()
		return true
	})

	return map[string]interface{}{
		"tracked_content_count": trackedCount,
		"total_accesses":        totalAccesses,
		"last_cleanup_time":     am.lastCleanupTime,
		"cleanup_interval":      am.cleanupInterval.String(),
	}
}

// getOrCreateTracker gets an existing tracker or creates a new one
func (am *AccessManager) getOrCreateTracker(contentID string) *models.AccessTracker {
	// Try to load existing tracker
	if value, exists := am.accessTrackers.Load(contentID); exists {
		return value.(*models.AccessTracker)
	}

	// Create new tracker
	tracker := &models.AccessTracker{
		ID:          contentID,
		AccessCount: 0,
	}

	// Store with LoadOrStore to handle race conditions
	if actual, loaded := am.accessTrackers.LoadOrStore(contentID, tracker); loaded {
		// Another goroutine created it first, use that one
		return actual.(*models.AccessTracker)
	}

	// We successfully stored our new tracker
	return tracker
}

// IsExpired checks if content has expired based on access count
// This is a convenience method that combines content expiration logic with access tracking
func (am *AccessManager) IsExpired(content *models.Content) bool {
	accessCount := am.GetAccessCount(content.ID)
	return content.IsExpired(accessCount)
}

// CreateContentWithAccess creates a ContentWithAccess struct that includes the current access count
// This is useful for API responses that need to include access count information
func (am *AccessManager) CreateContentWithAccess(content *models.Content) *models.ContentWithAccess {
	return &models.ContentWithAccess{
		Content:     content,
		AccessCount: am.GetAccessCount(content.ID),
	}
}

// CleanupExpiredTrackers removes access trackers for content that no longer exists
// This prevents memory leaks by cleaning up stale access tracking data
func (am *AccessManager) CleanupExpiredTrackers(existingContentIDs map[string]bool) int {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	removedCount := 0
	var toRemove []string

	// Collect IDs to remove (can't delete during Range)
	am.accessTrackers.Range(func(key, value interface{}) bool {
		contentID := key.(string)
		if !existingContentIDs[contentID] {
			toRemove = append(toRemove, contentID)
		}
		return true
	})

	// Remove stale trackers
	for _, contentID := range toRemove {
		am.accessTrackers.Delete(contentID)
		removedCount++
	}

	am.lastCleanupTime = time.Now()
	return removedCount
}

// ShouldRunCleanup returns true if it's time to run cleanup
func (am *AccessManager) ShouldRunCleanup() bool {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	return time.Since(am.lastCleanupTime) >= am.cleanupInterval
}

// SetCleanupInterval sets the cleanup interval
func (am *AccessManager) SetCleanupInterval(interval time.Duration) {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	am.cleanupInterval = interval
}
