package storage

import (
	"encoding/json"
	"fmt"

	"content-storage-server/pkg/models"

	badger "github.com/dgraph-io/badger/v4"
)

// List retrieves content items with pagination
func (s *BadgerStorage) List(limit, offset int) ([]*models.Content, error) {
	return s.ListWithFilter(limit, offset, nil)
}

// ListWithFilter retrieves content items with pagination and filtering
func (s *BadgerStorage) ListWithFilter(limit, offset int, filter *ContentFilter) ([]*models.Content, error) {
	contents := []*models.Content{}

	// Validate pagination parameters
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be non-negative")
	}

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true // We need to fetch values

		it := txn.NewIterator(opts)
		defer it.Close()

		skipped := 0
		collected := 0

		for it.Rewind(); it.Valid() && collected < limit; it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var content models.Content
				err := json.Unmarshal(val, &content)
				if err != nil {
					return fmt.Errorf("failed to unmarshal content for key '%s': %w", string(item.Key()), err)
				}

				// Apply filtering
				if !s.matchesFilter(&content, filter) {
					return nil // Skip this item
				}

				// Apply offset by skipping items
				if skipped < offset {
					skipped++
					return nil
				}

				// Collect the item
				contents = append(contents, &content)
				collected++
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return contents, nil
}

// CountWithFilter returns the number of content items matching the filter
func (s *BadgerStorage) CountWithFilter(filter *ContentFilter) int {
	count := 0

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true // We need to check content for filtering

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var content models.Content
				err := json.Unmarshal(val, &content)
				if err != nil {
					return err
				}

				if s.matchesFilter(&content, filter) {
					count++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return 0
	}

	return count
}
