package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"content-storage-server/pkg/models"
)

// TestRecoveryChecksumVerification tests that recovery verifies checksums
func TestRecoveryChecksumVerification(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "recovery-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test data
	testData := []byte("test recovery data")
	validChecksum := calculateChecksum(testData)
	invalidChecksum := "invalidchecksum1234567890123456789012345678901234567890123456789012345678"

	// Test valid checksum
	err = verifyChecksum(testData, validChecksum)
	if err != nil {
		t.Errorf("Valid checksum verification failed: %v", err)
	}

	// Test invalid checksum
	err = verifyChecksum(testData, invalidChecksum)
	if err == nil {
		t.Error("Expected error for invalid checksum, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("Expected checksum mismatch error, got: %v", err)
	}

	// Test empty checksum (should pass)
	err = verifyChecksum(testData, "")
	if err != nil {
		t.Errorf("Empty checksum should pass verification: %v", err)
	}
}

// TestEmergencyRecoveryWithCorruptedData tests recovery with corrupted pending items
func TestEmergencyRecoveryWithCorruptedData(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "corrupted-recovery-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage instance
	opts := badger.DefaultOptions(tempDir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	storage := &BadgerStorage{
		db:                     db,
		queuedBatch:            NewQueuedWriteBatch(db, QueuedWriteBatchOptions{QueueSize: 10}),
		emergencyRecoveryDir:   tempDir,
	}

	// Create corrupted recovery files
	timestamp := time.Now().Format("20060102150405")
	metadata := map[string]interface{}{
		"pending_checksum": "wrongchecksum",
		"queue_checksum":   "",
		"timestamp":        timestamp,
	}
	metadataData, _ := json.Marshal(metadata)
	metadataFile := filepath.Join(tempDir, "recovery-metadata-"+timestamp+".json")
	os.WriteFile(metadataFile, metadataData, 0644)

	// Create corrupted pending file
	pendingFile := filepath.Join(tempDir, "pending-items-"+timestamp+".json")
	os.WriteFile(pendingFile, []byte("corrupted data"), 0644)

	queueFile := filepath.Join(tempDir, "write-queue-"+timestamp+".json")
	os.WriteFile(queueFile, []byte("[]"), 0644)

	// Attempt recovery - should fail with checksum error
	err = storage.performEmergencyRecovery(pendingFile, queueFile, metadataFile, timestamp)
	if err == nil {
		t.Error("Expected error for corrupted data, got nil")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Expected checksum error, got: %v", err)
	}
}

// TestEmergencyRecoveryWithValidChecksum tests successful recovery with valid checksums
func TestEmergencyRecoveryWithValidChecksum(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "valid-recovery-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create storage instance
	opts := badger.DefaultOptions(tempDir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	storage := &BadgerStorage{
		db:                   db,
		queuedBatch:          NewQueuedWriteBatch(db, QueuedWriteBatchOptions{QueueSize: 10}),
		emergencyRecoveryDir: tempDir,
	}

	// Create valid test content
	expires := time.Now().Add(1 * time.Hour)
	testContent := &models.Content{
		ID:        "recovered-content",
		Type:      "text/plain",
		Data:      "recovered data",
		ExpiresAt: &expires,
	}

	// Create serializable task data
	type SerializableTask struct {
		Content       *models.Content `json:"content"`
		MarshaledData []byte          `json:"marshaled_data"`
	}

	tasks := []SerializableTask{
		{
			Content:       testContent,
			MarshaledData: nil,
		},
	}

	pendingData, _ := json.Marshal(tasks)
	pendingChecksum := calculateChecksum(pendingData)

	// Create valid recovery files
	timestamp := time.Now().Format("20060102150405")
	metadata := map[string]interface{}{
		"pending_checksum": pendingChecksum,
		"queue_checksum":   "",
		"timestamp":        timestamp,
	}
	metadataData, _ := json.Marshal(metadata)
	metadataFile := filepath.Join(tempDir, "recovery-metadata-"+timestamp+".json")
	os.WriteFile(metadataFile, metadataData, 0644)

	pendingFile := filepath.Join(tempDir, "pending-items-"+timestamp+".json")
	os.WriteFile(pendingFile, pendingData, 0644)

	queueFile := filepath.Join(tempDir, "write-queue-"+timestamp+".json")
	os.WriteFile(queueFile, []byte("[]"), 0644)

	// Attempt recovery - should succeed
	err = storage.performEmergencyRecovery(pendingFile, queueFile, metadataFile, timestamp)
	if err != nil {
		t.Errorf("Recovery with valid checksum should succeed: %v", err)
	}
}

// TestRestorePendingItemsWithInvalidData tests restore with invalid pending items
func TestRestorePendingItemsWithInvalidData(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "restore-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	storage := &BadgerStorage{
		db:          db,
		queuedBatch: NewQueuedWriteBatch(db, QueuedWriteBatchOptions{QueueSize: 10}),
	}

	// Test with invalid JSON
	invalidData := []byte("not valid json")
	err = storage.restorePendingItems(invalidData)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}

	// Test with nil queued batch
	nilBatchStorage := &BadgerStorage{db: db, queuedBatch: nil}
	err = nilBatchStorage.restorePendingItems([]byte("[]"))
	if err != nil {
		t.Errorf("Restore with nil batch should not error: %v", err)
	}
}

// TestRestoreWriteQueueWithInvalidData tests restore with invalid queue data
func TestRestoreWriteQueueWithInvalidData(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "restore-queue-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	storage := &BadgerStorage{
		db:          db,
		queuedBatch: NewQueuedWriteBatch(db, QueuedWriteBatchOptions{QueueSize: 10}),
	}

	// Test with invalid JSON
	invalidData := []byte("not valid json")
	err = storage.restoreWriteQueue(invalidData)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}

	// Test with nil queued batch
	nilBatchStorage := &BadgerStorage{db: db, queuedBatch: nil}
	err = nilBatchStorage.restoreWriteQueue([]byte("[]"))
	if err != nil {
		t.Errorf("Restore with nil batch should not error: %v", err)
	}
}

// TestRecoveryMetadataExtraction tests timestamp extraction from filenames
func TestRecoveryMetadataExtraction(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
		prefix   string
		suffix   string
		expected string
	}{
		{
			name:     "valid timestamp",
			filename: "pending-items-20250210153045.json",
			prefix:   "pending-items-",
			suffix:   ".json",
			expected: "20250210153045",
		},
		{
			name:     "different prefix",
			filename: "write-queue-20250210153045.json",
			prefix:   "write-queue-",
			suffix:   ".json",
			expected: "20250210153045",
		},
		{
			name:     "too short",
			filename: "short.json",
			prefix:   "pending-items-",
			suffix:   ".json",
			expected: "",
		},
		{
			name:     "empty",
			filename: "",
			prefix:   "pending-items-",
			suffix:   ".json",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractTimestamp(tc.filename, tc.prefix, tc.suffix)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// TestChecksumWithEmptyData tests checksum calculation with edge cases
func TestChecksumWithEmptyData(t *testing.T) {
	// Empty data should still produce valid checksum
	emptyData := []byte("")
	checksum := calculateChecksum(emptyData)

	if checksum == "" {
		t.Error("Empty data should produce checksum")
	}
	if len(checksum) != 64 {
		t.Errorf("Expected checksum length 64, got %d", len(checksum))
	}

	// Verify it's reproducible
	checksum2 := calculateChecksum(emptyData)
	if checksum != checksum2 {
		t.Error("Checksum should be reproducible for empty data")
	}
}

// TestArchiveRecoveredFiles tests file archiving after recovery
func TestArchiveRecoveredFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "archive-recovery-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	storage := &BadgerStorage{
		db:                   db,
		emergencyRecoveryDir: tempDir,
	}

	timestamp := "20250210153045"

	// Create test files in temp dir
	pendingFile := filepath.Join(tempDir, "pending-items-"+timestamp+".json")
	queueFile := filepath.Join(tempDir, "write-queue-"+timestamp+".json")
	metadataFile := filepath.Join(tempDir, "recovery-metadata-"+timestamp+".json")

	os.WriteFile(pendingFile, []byte("{}"), 0644)
	os.WriteFile(queueFile, []byte("{}"), 0644)
	os.WriteFile(metadataFile, []byte("{}"), 0644)

	// Archive files
	storage.archiveRecoveredFiles(pendingFile, queueFile, metadataFile, timestamp)

	// Verify files are moved to processed directory
	processedDir := filepath.Join(tempDir, "processed")

	// Check that original files don't exist
	if _, err := os.Stat(pendingFile); !os.IsNotExist(err) {
		t.Error("Pending file should be moved after archiving")
	}
	if _, err := os.Stat(queueFile); !os.IsNotExist(err) {
		t.Error("Queue file should be moved after archiving")
	}
	if _, err := os.Stat(metadataFile); !os.IsNotExist(err) {
		t.Error("Metadata file should be moved after archiving")
	}

	// Check that files exist in processed directory
	newPendingFile := filepath.Join(processedDir, "pending-items-"+timestamp+".json")
	if _, err := os.Stat(newPendingFile); os.IsNotExist(err) {
		t.Error("Pending file should exist in processed directory")
	}

	newQueueFile := filepath.Join(processedDir, "write-queue-"+timestamp+".json")
	if _, err := os.Stat(newQueueFile); os.IsNotExist(err) {
		t.Error("Queue file should exist in processed directory")
	}

	newMetadataFile := filepath.Join(processedDir, "recovery-metadata-"+timestamp+".json")
	if _, err := os.Stat(newMetadataFile); os.IsNotExist(err) {
		t.Error("Metadata file should exist in processed directory")
	}
}
