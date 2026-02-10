package storage

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"content-storage-server/pkg/models"
)

// TestEmergencyDataVerificationChecksum tests that SHA256 checksums are correctly
// calculated and included in emergency serialization data
func TestEmergencyDataVerificationChecksum(t *testing.T) {
	// Create temporary directory for BadgerDB
	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	// Create queued write batch
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    10,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	// Add test content to queue
	expires := time.Now().Add(1 * time.Hour)
	testContent := &models.Content{
		ID:        "test-content-1",
		Type:      "text/plain",
		Data:      "test data for checksum verification",
		ExpiresAt: &expires,
	}

	err = qwb.QueueWrite(testContent)
	if err != nil {
		t.Fatalf("Failed to queue write: %v", err)
	}

	// Serialize pending tasks
	data, checksum, err := qwb.SerializePendingTasks()
	if err != nil {
		t.Fatalf("Failed to serialize pending tasks: %v", err)
	}

	// Verify checksum is not empty
	if checksum == "" {
		t.Error("Expected non-empty checksum, got empty string")
	}

	// Verify checksum format (SHA256 should be 64 hex characters)
	if len(checksum) != 64 {
		t.Errorf("Expected checksum length of 64, got %d", len(checksum))
	}

	// Verify all characters are valid hex
	for _, c := range checksum {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Checksum contains invalid hex character: %c", c)
		}
	}

	// Verify checksum is reproducible
	data2, checksum2, err := qwb.SerializePendingTasks()
	if err != nil {
		t.Fatalf("Failed to serialize pending tasks second time: %v", err)
	}

	if checksum != checksum2 {
		t.Error("Checksum should be reproducible for same data")
	}

	if string(data) != string(data2) {
		t.Error("Serialized data should be reproducible")
	}
}

// TestChecksumVerificationValidData tests that valid data with matching checksum passes verification
func TestChecksumVerificationValidData(t *testing.T) {
	testData := []byte("test data for verification")
	testChecksum := calculateChecksum(testData)

	err := verifyChecksum(testData, testChecksum)
	if err != nil {
		t.Errorf("Expected no error for valid checksum, got: %v", err)
	}
}

// TestChecksumVerificationInvalidData tests that corrupted data fails verification
func TestChecksumVerificationInvalidData(t *testing.T) {
	testData := []byte("original data")
	testChecksum := calculateChecksum(testData)

	// Corrupt the data
	corruptedData := []byte("corrupted data")

	err := verifyChecksum(corruptedData, testChecksum)
	if err == nil {
		t.Error("Expected error for corrupted data, got nil")
	}

	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("Expected checksum mismatch error, got: %v", err)
	}
}

// TestChecksumVerificationEmptyChecksum tests that verification passes when checksum is empty
func TestChecksumVerificationEmptyChecksum(t *testing.T) {
	testData := []byte("test data")

	err := verifyChecksum(testData, "")
	if err != nil {
		t.Errorf("Expected no error when checksum is empty, got: %v", err)
	}
}

// TestEmergencySerializationConsistency tests that serialization produces consistent data
func TestEmergencySerializationConsistency(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    10,
		BatchSize:    5,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	// Add multiple test contents
	expires := time.Now().Add(1 * time.Hour)
	contents := []*models.Content{
		{
			ID:        "content-1",
			Type:      "text/plain",
			Data:      "data 1",
			ExpiresAt: &expires,
		},
		{
			ID:        "content-2",
			Type:      "application/json",
			Data:      `{"key": "value"}`,
			ExpiresAt: &expires,
		},
	}

	for _, content := range contents {
		err = qwb.QueueWrite(content)
		if err != nil {
			t.Fatalf("Failed to queue write: %v", err)
		}
	}

	// Serialize and verify
	data, checksum, err := qwb.SerializePendingTasks()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	// Verify data can be unmarshaled
	type SerializableTask struct {
		Content       *models.Content `json:"content"`
		MarshaledData []byte          `json:"marshaled_data"`
	}

	var tasks []SerializableTask
	err = json.Unmarshal(data, &tasks)
	if err != nil {
		t.Errorf("Failed to unmarshal serialized data: %v", err)
	}

	// Verify all contents are present
	if len(tasks) != len(contents) {
		t.Errorf("Expected %d tasks, got %d", len(contents), len(tasks))
	}

	// Verify checksum matches data
	calculatedChecksum := calculateChecksum(data)
	if calculatedChecksum != checksum {
		t.Error("Checksum does not match calculated checksum")
	}
}

// TestWriteQueueSerializationWithChecksum tests write queue serialization includes checksum
func TestWriteQueueSerializationWithChecksum(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil // Disable logger for tests
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    10,
		BatchSize:    2, // Small batch size to trigger batching
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	// Fill queue with items
	expires := time.Now().Add(1 * time.Hour)
	for i := 0; i < 5; i++ {
		content := &models.Content{
			ID:        "queue-content-" + string(rune('0'+i)),
			Type:      "text/plain",
			Data:      "data " + string(rune('0'+i)),
			ExpiresAt: &expires,
		}
		err = qwb.QueueWrite(content)
		if err != nil {
			t.Fatalf("Failed to queue write: %v", err)
		}
	}

	// Serialize write queue
	data, checksum, err := qwb.SerializeWriteQueue()
	if err != nil {
		t.Fatalf("Failed to serialize write queue: %v", err)
	}

	// Verify checksum
	if checksum == "" {
		t.Error("Expected non-empty checksum for write queue serialization")
	}

	// Verify checksum format
	if len(checksum) != 64 {
		t.Errorf("Expected checksum length of 64, got %d", len(checksum))
	}

	// Verify data is valid JSON
	if !json.Valid(data) {
		t.Error("Serialized write queue data is not valid JSON")
	}
}

// TestCalculateChecksumDeterministic tests that calculateChecksum is deterministic
func TestCalculateChecksumDeterministic(t *testing.T) {
	testData := []byte("deterministic test data")

	checksum1 := calculateChecksum(testData)
	checksum2 := calculateChecksum(testData)

	if checksum1 != checksum2 {
		t.Error("calculateChecksum should be deterministic")
	}

	// Different data should produce different checksum
	differentData := []byte("different test data")
	checksum3 := calculateChecksum(differentData)

	if checksum1 == checksum3 {
		t.Error("Different data should produce different checksums")
	}
}

// TestChecksumCollisionResistance tests that similar data produces different checksums
func TestChecksumCollisionResistance(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte("")},
		{"single char", []byte("a")},
		{"short text", []byte("hello")},
		{"long text", []byte(strings.Repeat("test", 100))},
		{"binary", []byte{0x00, 0x01, 0x02, 0xFF}},
	}

	checksums := make(map[string]string)
	for _, tc := range testCases {
		checksum := calculateChecksum(tc.data)
		if existing, exists := checksums[checksum]; exists {
			t.Errorf("Checksum collision detected: %s produced same checksum as %s", tc.name, existing)
		}
		checksums[checksum] = tc.name
	}
}
