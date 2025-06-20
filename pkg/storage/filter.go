package storage

import (
	"content-storage-server/pkg/models"
)

// matchesFilter checks if content matches the given filter criteria
func (s *BadgerStorage) matchesFilter(content *models.Content, filter *ContentFilter) bool {
	// Get current access count for expiration check
	accessCount := s.accessManager.GetAccessCount(content.ID)

	// If no filter provided, include all non-expired content by default
	if filter == nil {
		return !content.IsExpired(accessCount)
	}

	// Check expiration status
	if !filter.IncludeExpired && content.IsExpired(accessCount) {
		return false
	}

	// Check content type filter
	if filter.ContentType != "" && content.Type != filter.ContentType {
		return false
	}

	// Check tag filter
	if filter.Tag != "" && content.Tag != filter.Tag {
		return false
	}

	// Check creation date filters
	// CreatedAfter is inclusive (content created at or after the specified time)
	if filter.CreatedAfter != nil && content.CreatedAt.Before(*filter.CreatedAfter) {
		return false
	}

	// CreatedBefore is exclusive (content created before the specified time)
	if filter.CreatedBefore != nil && !content.CreatedAt.Before(*filter.CreatedBefore) {
		return false
	}

	return true
}
