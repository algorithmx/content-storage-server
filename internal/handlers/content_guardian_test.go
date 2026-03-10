package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

	// Create test content with high access limit (effectively unlimited for this test)
	contentID := "test-get-increments"
	content := &models.Content{
		ID:          contentID,
		Type:        "text/plain",
		Data:        "test data",
		CreatedAt:   time.Now(),
		AccessLimit: 1000, // High limit for testing access count increments
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
// With access_limit=3 and check-before-increment logic:
// - Accesses 1,2,3 succeed (count 0,1,2 < 3)
// - Access 4 fails (count 3 >= 3)
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

	// Access content 3 times - all should succeed (exactly 3 accesses allowed)
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(contentID)

		err := handler.GetContent(c)
		require.NoError(t, err)

		// All 3 accesses should succeed
		assert.Equal(t, http.StatusOK, rec.Code,
			"Access %d/%d should return 200", i, 3)
	}

	// 4th access should fail (limit reached)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(contentID)

	err = handler.GetContent(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusGone, rec.Code,
		"4th access should return 410 Gone when limit exceeded, got %d", rec.Code)
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
		{"access limit zero (no access)", 0, 0, true},
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

// TestDeleteNonExistentContentReturns404 verifies Issue #4 fix:
// Deleting non-existent content should return 404, not 200
func TestDeleteNonExistentContentReturns404(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Try to delete a non-existent content ID
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/content/non-existent-id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("non-existent-id")

	err := handler.DeleteContent(c)
	require.NoError(t, err)

	// Should return 404 Not Found
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"Deleting non-existent content should return 404 Not Found, got %d. "+
		"If 200, the Delete operation is not checking existence properly", rec.Code)
}

// TestDeleteExistingContentSucceeds verifies that deleting existing content works
func TestDeleteExistingContentSucceeds(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create and store content
	contentID := "test-delete-existing"
	content := &models.Content{
		ID:        contentID,
		Type:      "text/plain",
		Data:      "test data",
		CreatedAt: time.Now(),
	}

	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Verify content exists
	_, err = badgerStorage.GetReadOnly(contentID)
	require.NoError(t, err)

	// Delete the content
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/content/"+contentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(contentID)

	err = handler.DeleteContent(c)
	require.NoError(t, err)

	// Should return 200 OK
	assert.Equal(t, http.StatusOK, rec.Code,
		"Deleting existing content should return 200 OK, got %d", rec.Code)

	// Verify content no longer exists
	_, err = badgerStorage.GetReadOnly(contentID)
	assert.Error(t, err, "Content should not exist after deletion")
}

// TestContentTypeValidationInPerformanceMode verifies Issue #6 fix:
// Content type validation should be enforced even in performance mode
func TestContentTypeValidationInPerformanceMode(t *testing.T) {
	handler, _, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// The test config has AllowedContentTypes: []string{"text/plain", "application/json"}
	// So "invalid/type" should be rejected

	body := `{"id":"test-invalid-type","type":"invalid/type","data":"test data"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.StoreContent(c)
	require.NoError(t, err)

	// Should return 400 Bad Request (or 415 Unsupported Media Type if implemented)
	// The current implementation returns 400 with validation error
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"Invalid content type should be rejected with 400 Bad Request, got %d. "+
		"If 202, content type validation is not being enforced in performance mode", rec.Code)

	// Verify response contains content type error
	responseBody := rec.Body.String()
	assert.Contains(t, responseBody, "not in allowed types",
		"Response should indicate content type is not allowed")
}

// TestAllowedContentTypes passes when using allowed types
func TestAllowedContentTypes(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
		shouldPass  bool
	}{
		{"text/plain", "text/plain", true},
		{"application/json", "application/json", true},
		{"text/html", "text/html", false},
		{"image/png", "image/png", false},
		{"application/xml", "application/xml", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, _, cleanup := setupTestHandler(t)
			defer cleanup()

			e := echo.New()

			body := fmt.Sprintf(`{"id":"test-allowed-%s","type":"%s","data":"test data"}`,
				strings.ReplaceAll(tc.contentType, "/", "-"), tc.contentType)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/content", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.StoreContent(c)
			require.NoError(t, err)

			if tc.shouldPass {
				assert.Equal(t, http.StatusAccepted, rec.Code,
					"Allowed content type '%s' should be accepted, body: %s", tc.contentType, rec.Body.String())
			} else {
				assert.Equal(t, http.StatusBadRequest, rec.Code,
					"Disallowed content type '%s' should be rejected", tc.contentType)
			}
		})
	}
}

// TestAccessLimitZeroReturns410 verifies that content with access_limit=0
// returns 410 Gone immediately (no access allowed)
func TestAccessLimitZeroReturns410(t *testing.T) {
	handler, badgerStorage, cleanup := setupTestHandler(t)
	defer cleanup()

	e := echo.New()

	// Create content with access_limit=0 (no access allowed)
	contentID := "test-zero-access-limit"
	content := &models.Content{
		ID:          contentID,
		Type:        "text/plain",
		Data:        "test data",
		CreatedAt:   time.Now(),
		AccessLimit: 0, // No access allowed
	}

	// Store content
	err := badgerStorage.StoreSync(content)
	require.NoError(t, err)

	// Try to get the content - should return 410 Gone immediately
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+contentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(contentID)

	err = handler.GetContent(c)
	require.NoError(t, err)

	// Should return 410 Gone because access_limit=0 means no access allowed
	assert.Equal(t, http.StatusGone, rec.Code,
		"Content with access_limit=0 should return 410 Gone immediately, got %d. "+
			"If 200, the access_limit=0 is being treated as unlimited instead of no-access", rec.Code)
}
