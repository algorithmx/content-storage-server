package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"content-storage-server/pkg/config"
	"content-storage-server/pkg/logger"
	"content-storage-server/pkg/models"
	"content-storage-server/pkg/storage"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// setupTestHandler creates a ContentHandler with a temporary storage for testing
func setupTestHandler(t *testing.T) (*ContentHandler, *storage.BadgerStorage, func()) {
	// Create temporary directory for test storage
	tempDir, err := os.MkdirTemp("", "handler-test-*")
	require.NoError(t, err)

	// Create config
	cfg := &config.Config{
		MaxContentSize:      10 * 1024 * 1024,
		MaxIDLength:         255,
		MaxTypeLength:       100,
		MaxPaginationLimit:  1000,
		MaxAccessLimit:      1000000,
		PerformanceMode:     true,
		AllowedContentTypes: []string{"text/plain", "application/json"},
	}

	// Create logger
	log, err := logger.NewLogger(logger.Config{
		Level: zapcore.ErrorLevel,
	})
	require.NoError(t, err)

	// Create storage
	badgerOpts := storage.BadgerOptions{
		DataDir:                tempDir,
		WriteQueueSize:         100,
		WriteQueueBatchSize:    10,
		WriteQueueBatchTimeout: 100 * time.Millisecond,
	}
	badgerStorage, err := storage.NewBadgerStorage(badgerOpts)
	require.NoError(t, err)

	// Create handler
	handler := NewContentHandler(badgerStorage, log, cfg)

	cleanup := func() {
		badgerStorage.Close()
		os.RemoveAll(tempDir)
	}

	return handler, badgerStorage, cleanup
}

// TestAccessCountNotIncrementedByStatusCheck verifies Issue #1 fix:
// GetContentStatus should NOT increment the access count
func TestAccessCountNotIncrementedByStatusCheck(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create test content
	contentID := "test-access-count"
	content := &models.Content{
		ID:          contentID,
		Type:        "text/plain",
		Data:        "test data",
		CreatedAt:   time.Now(),
		AccessLimit: 10,
	}

	// Store content synchronously to ensure it's persisted
	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Poll status multiple times (this should NOT increment access count)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID+"/status", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(contentID)

		err := handler.GetContentStatus(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Get the content - this SHOULD increment access count exactly once
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(contentID)

	err = handler.GetContent(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify access count is exactly 1 (not 6 = 5 status checks + 1 get)
	accessCount := badgerStorage.GetAccessCount(contentID)
	assert.Equal(t, int64(1), accessCount,
		"Access count should be 1 after first retrieval, but got %d. "+
		"Status checks may be incorrectly incrementing the count.", accessCount)
}

// TestAccessCountIncrementedByGetContent verifies that GetContent properly increments access count
func TestAccessCountIncrementedByGetContent(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create test content
	contentID := "test-get-increments"
	content := &models.Content{
		ID:        contentID,
		Type:      "text/plain",
		Data:      "test data",
		CreatedAt: time.Now(),
	}

	// Store content
	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Get content multiple times
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(contentID)

		err := handler.GetContent(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify access count equals iteration count
		accessCount := badgerStorage.GetAccessCount(contentID)
		assert.Equal(t, int64(i), accessCount,
			"Access count should be %d after %d retrievals", i, i)
	}
}

// TestExpiredContentReturns410 verifies Issue #2 fix:
// Expired content should return HTTP 410 Gone, not 500
func TestExpiredContentReturns410(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create content that expires in the past (already expired)
	pastTime := time.Now().Add(-1 * time.Hour)
	contentID := "test-expired-content"
	content := &models.Content{
		ID:        contentID,
		Type:      "text/plain",
		Data:      "test data",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &pastTime,
	}

	// Store content
	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Set access count to 0 (not expired by access count)
	badgerStorage.SetAccessCount(contentID, 0)

	// Try to get the expired content
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(contentID)

	err = handler.GetContent(c)
	require.NoError(t, err)

	// Should return 410 Gone, not 500
	assert.Equal(t, http.StatusGone, rec.Code,
		"Expired content should return 410 Gone, got %d. "+
		"If 500, the handler is not properly handling ErrContentExpired", rec.Code)
}

// TestAccessLimitEnforcement verifies that content expires when access limit is reached
func TestAccessLimitEnforcement(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create content with access limit of 3
	contentID := "test-access-limit"
	content := &models.Content{
		ID:          contentID,
		Type:        "text/plain",
		Data:        "test data",
		CreatedAt:   time.Now(),
		AccessLimit: 3,
	}

	// Store content
	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Access content 3 times (should succeed each time until limit is reached)
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(contentID)

		err := handler.GetContent(c)
		require.NoError(t, err)

		if i < 3 {
			// Should succeed for accesses 1 and 2
			assert.Equal(t, http.StatusOK, rec.Code,
				"Access %d/%d should return 200", i, 3)
		} else {
			// 3rd access should return 410 (limit reached)
			assert.Equal(t, http.StatusGone, rec.Code,
				"Access %d/%d should return 410 Gone when limit reached, got %d. "+
				"If 500, the handler is not properly handling ErrContentExpired", i, 3, rec.Code)
		}
	}
}

// TestNotFoundContentReturns404 verifies that non-existent content returns 404
func TestNotFoundContentReturns404(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Try to get non-existent content
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/non-existent-id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("non-existent-id")

	err := handler.GetContent(c)
	require.NoError(t, err)

	// Should return 404 Not Found
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"Non-existent content should return 404 Not Found, got %d", rec.Code)
}

// TestContentNotFoundStatusEndpoint verifies that status check returns 404 for non-existent content
func TestContentNotFoundStatusEndpoint(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Try to get status of non-existent content
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/non-existent-id/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("non-existent-id")

	err := handler.GetContentStatus(c)
	require.NoError(t, err)

	// Should return 404 Not Found
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"Status check for non-existent content should return 404, got %d", rec.Code)
}

// TestIsExpiredLogic verifies the IsExpired function handles edge cases correctly
func TestIsExpiredLogic(t *testing.T) {
	tests := []struct {
		name        string
		accessLimit int
		accessCount int64
		expired     bool
	}{
		{"access count below limit", 3, 2, false},
		{"access count at limit", 3, 3, true},
		{"access count above limit", 3, 4, true},
		{"access limit zero (no limit)", 0, 100, false},
		{"access count zero", 5, 0, false},
		{"access count one below", 5, 4, false},
		{"access count exactly at limit", 5, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := &models.Content{
				ID:          "test",
				Type:        "text/plain",
				Data:        "test",
				CreatedAt:   time.Now(),
				AccessLimit: tt.accessLimit,
			}

			result := content.IsExpired(tt.accessCount)
			assert.Equal(t, tt.expired, result,
				"IsExpired(%d) with limit %d should return %v, got %v",
				tt.accessCount, tt.accessLimit, tt.expired, result)
		})
	}
}

// TestTimeExpiredContentReturns410 verifies that time-expired content returns 410
func TestTimeExpiredContentReturns410(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create content that expired 1 second ago
	expiredTime := time.Now().Add(-1 * time.Second)
	contentID := "test-time-expired"
	content := &models.Content{
		ID:        contentID,
		Type:      "text/plain",
		Data:      "test data",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &expiredTime,
	}

	// Store content
	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Wait to ensure expiration
	time.Sleep(100 * time.Millisecond)

	// Try to get the expired content
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(contentID)

	err = handler.GetContent(c)
	require.NoError(t, err)

	// Should return 410 Gone, not 500
	assert.Equal(t, http.StatusGone, rec.Code,
		"Time-expired content should return 410 Gone, got %d. "+
		"If 500, the handler is not properly handling ErrContentExpired", rec.Code)
}
