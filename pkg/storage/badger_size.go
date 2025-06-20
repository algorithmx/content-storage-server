package storage

import (
	"fmt"

	badger "github.com/dgraph-io/badger/v4"
)

// GetDatabaseSize returns the actual size of the data stored in the BadgerDB database in bytes
// This method calculates the real data size by iterating through all key-value pairs,
// not the allocated file size which can be much larger due to pre-allocation
func (s *BadgerStorage) GetDatabaseSize() (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var totalDataSize int64

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need key sizes for now
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()

			// Add key size
			totalDataSize += int64(item.KeySize())

			// Add value size
			totalDataSize += int64(item.ValueSize())
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to calculate database size: %w", err)
	}

	return totalDataSize, nil
}

// GetDatabaseFileSize returns the allocated file sizes of the BadgerDB database in bytes
// This represents the disk space used by BadgerDB files, which may be larger than actual data
func (s *BadgerStorage) GetDatabaseFileSize() (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	// Use BadgerDB's built-in Size() method to get LSM and value log file sizes
	lsmSize, vlogSize := s.db.Size()
	totalFileSize := lsmSize + vlogSize

	return totalFileSize, nil
}
