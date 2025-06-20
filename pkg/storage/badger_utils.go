package storage

import (
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// applyBackoffDelay applies exponential backoff delay for transaction retries
func (s *BadgerStorage) applyBackoffDelay(attempt int) {
	// Exponential backoff: wait 2^attempt milliseconds (max 32ms)
	// Start with 1ms, then 2ms, 4ms, 8ms, 16ms, 32ms
	backoffTime := time.Duration(1<<uint(attempt)) * time.Millisecond
	if backoffTime > 32*time.Millisecond {
		backoffTime = 32 * time.Millisecond
	}
	time.Sleep(backoffTime)
}

// isRetryableTransactionError checks if an error is a retryable transaction conflict
func (s *BadgerStorage) isRetryableTransactionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for BadgerDB transaction conflict errors
	// BadgerDB v4 returns "Transaction Conflict. Please retry" for transaction conflicts
	errStr := err.Error()
	return strings.Contains(errStr, "Transaction Conflict") ||
		   strings.Contains(errStr, "transaction conflict") ||
		   err == badger.ErrConflict
}

// Count returns the number of content items in storage
func (s *BadgerStorage) count_items_in_db() int {
	count := 0

	// Iterate through all keys to count them
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only care about keys

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			count++
		}

		return nil
	})

	if err != nil {
		return 0
	}

	return count
}
