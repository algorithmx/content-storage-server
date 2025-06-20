package storage

import (
	"content-storage-server/pkg/models"
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// Count returns the number of content items in storage (including pending writes)
func (s *BadgerStorage) Count() int {
	dbCount := s.count_items_in_db()

	// Only count items that are actually persisted in the database
	// Pending writes in the queue are not yet considered part of the durable storage
	return dbCount
}

// Store saves content to storage with conflict resolution and retry logic
func (s *BadgerStorage) Store(content *models.Content) error {
	if s.queuedBatch == nil {
		return ErrStorageNotReady
	}
	return s.queuedBatch.QueueWrite(content)
}

// StoreSync saves content to storage synchronously (waits for completion)
func (s *BadgerStorage) StoreSync(content *models.Content) error {
	if s.queuedBatch == nil {
		return ErrStorageNotReady
	}
	return s.queuedBatch.QueueWriteSync(content)
}

// getContentFromDB is a helper function that retrieves content from the database
// with optional access count increment and queue-aware read consistency
// Read-write consistency is guaranteed by the queued write system.
func (s *BadgerStorage) getContentFromDB(id string, incrementAccess bool) (*models.Content, error) {
	// Check if content is in the write queue (pending write)
	if s.queuedBatch != nil && s.queuedBatch.IsContentInQueue(id) {
		// Wait for the pending write to complete to ensure read-after-write consistency.
		// A timeout is added to prevent indefinite blocking.
		// The request sender must pay for a price of a potential delay in getting the premature content.
		waitErr := s.queuedBatch.WaitForContentWrite(id, 10*time.Second)
		if waitErr != nil {
			// If waiting fails (e.g., timeout), return the error. The client might retry based on this.
			return nil, fmt.Errorf("failed to get content %s due to pending write: %w", id, waitErr)
		}
		// After waiting, the content should now be in the database, or the write failed.
		// We proceed to read from the DB, and if the write failed, the DB read will return ErrContentNotFound.
	}

	var content models.Content

	// Use View transaction for read-only database access (no more Update transactions needed)
	err := s.db.View(func(txn *badger.Txn) error {
		return s.retrieveAndProcessContent(txn, id, &content)
	})

	if err != nil {
		return nil, err
	}

	// Handle access count increment after successful retrieval
	if incrementAccess {
		// Increment access count first
		newAccessCount := s.accessManager.IncrementAccess(id)

		// Check expiration after incrementing (this allows exactly AccessLimit accesses)
		if content.IsExpired(newAccessCount) {
			return nil, ErrContentExpired
		}
	}

	return &content, nil
}

// retrieveAndProcessContent handles the common logic for retrieving content from database
func (s *BadgerStorage) retrieveAndProcessContent(txn *badger.Txn, id string, content *models.Content) error {
	item, err := txn.Get([]byte(id))
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return ErrContentNotFound
		}
		return err
	}

	// Get the value and unmarshal
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, content)
	})
	if err != nil {
		return fmt.Errorf("failed to unmarshal content: %w", err)
	}

	// Note: Expiration check is now handled in the calling function
	// after access count is considered
	return nil
}

// Get retrieves content by ID and atomically increments access count
func (s *BadgerStorage) Get(id string) (*models.ContentWithAccess, error) {
	content, err := s.getContentFromDB(id, true)
	if err != nil {
		return nil, err
	}

	// Create ContentWithAccess that includes current access count
	return s.accessManager.CreateContentWithAccess(content), nil
}

// Delete removes content from storage with queue-aware consistency
func (s *BadgerStorage) Delete(id string) error {
	// Check if content is currently being written to the queue
	if s.queuedBatch != nil && s.queuedBatch.IsContentInQueue(id) {
		// Wait for the pending write to complete to ensure delete-after-write consistency.
		// A timeout is added to prevent indefinite blocking.
		waitErr := s.queuedBatch.WaitForContentWrite(id, 10*time.Second)
		if waitErr != nil {
			// If waiting fails (e.g., timeout), return the error.
			return fmt.Errorf("failed to delete content %s due to pending write: %w", id, waitErr)
		}
	}

	// Retry logic for transaction conflicts (max 5 attempts)
	maxRetries := 5
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Proceed with direct database deletion
		err = s.db.Update(func(txn *badger.Txn) error {
			deleteErr := txn.Delete([]byte(id))
			if deleteErr == badger.ErrKeyNotFound {
				return ErrContentNotFound
			}
			return deleteErr
		})

		// Check if this is a transaction conflict that we should retry
		if err != nil && s.isRetryableTransactionError(err) && attempt < maxRetries-1 {
			// Apply exponential backoff delay before retrying
			s.applyBackoffDelay(attempt)
			continue
		}

		// Either success, non-retryable error, or max retries reached
		break
	}

	// If deletion was successful, also remove access tracking
	if err == nil {
		s.accessManager.RemoveAccess(id)
	}

	return err
}

// GetAllContentIDs returns a map of all existing content IDs for cleanup purposes
func (s *BadgerStorage) GetAllContentIDs() (map[string]bool, error) {
	contentIDs := make(map[string]bool)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need keys
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			contentIDs[key] = true
		}
		return nil
	})

	return contentIDs, err
}

// GetReadOnly performs read-only operations without access count increment
func (s *BadgerStorage) GetReadOnly(id string) (*models.Content, error) {
	return s.getContentFromDB(id, false)
}

// GetAccessCount returns the current access count for a content ID
func (s *BadgerStorage) GetAccessCount(id string) int64 {
	return s.accessManager.GetAccessCount(id)
}

// SetAccessCount sets the access count for a content ID to a specific value
func (s *BadgerStorage) SetAccessCount(id string, count int64) {
	s.accessManager.SetAccessCount(id, count)
}

// GetAccessManager returns the access manager for direct access
func (s *BadgerStorage) GetAccessManager() *AccessManager {
	return s.accessManager
}

// RemoveAccessTracking removes access tracking for a content ID
func (s *BadgerStorage) RemoveAccessTracking(id string) {
	s.accessManager.RemoveAccess(id)
}
