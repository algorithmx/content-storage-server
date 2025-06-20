package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"content-storage-server/pkg/config"
	"content-storage-server/pkg/models"
	"content-storage-server/pkg/storage"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ContentHandler handles HTTP requests for content storage operations
type ContentHandler struct {
	storage   storage.Storage
	logger    *zap.Logger
	validator *RequestValidator
	cfg       *config.Config
}

// NewContentHandler creates a new content handler
func NewContentHandler(storage storage.Storage, appLogger *zap.Logger, cfg *config.Config) *ContentHandler {
	return &ContentHandler{
		storage:   storage,
		logger:    appLogger,
		validator: NewRequestValidator(cfg),
		cfg:       cfg,
	}
}

// logValidationError logs validation errors only if validation logging is enabled
func (h *ContentHandler) logValidationError(msg string, fields ...zap.Field) {
	if h.cfg.EnableValidationLogging {
		h.logger.Warn(msg, fields...)
	}
}

// logError logs errors only if error logging is enabled
func (h *ContentHandler) logError(msg string, fields ...zap.Field) {
	if h.cfg.EnableErrorLogging {
		h.logger.Error(msg, fields...)
	}
}

// StoreContent handles POST /api/v1/content
// @Summary Store new content with sequential write processing
// @Description Store new content in the system using sequential write queue for data consistency and emergency shutdown capabilities
// @Description
// @Description ## Configuration Dependencies:
// @Description - **MAX_CONTENT_SIZE**: Validates content size (default: 10MB)
// @Description - **ALLOWED_CONTENT_TYPES**: Validates request content type (default: application/json,text/plain)
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **PERFORMANCE_MODE**: Enables optimized validation when true (default: true)
// @Description
// @Description ## Sequential Write Processing:
// @Description - Content is queued for sequential processing to ensure data consistency
// @Description - Returns HTTP 202 (Accepted) immediately after validation and queuing
// @Description - Use GET /api/v1/content/{id}/status to check storage completion
// @Description - Emergency shutdown capabilities preserve queued content
// @Description
// @Description ## Validation Rules:
// @Description - ID: Required, max 255 chars, alphanumeric with dots/hyphens/underscores, no dangerous path patterns
// @Description - Data: Required, non-empty, size limited by MAX_CONTENT_SIZE
// @Description - Type: Required, must match ALLOWED_CONTENT_TYPES, follows MIME type format
// @Description - Tag: Optional, max 100 chars, alphanumeric with dots/hyphens/underscores
// @Description - AccessLimit: Optional, 0-1,000,000, content expires after this many retrievals
// @Description - ExpiresAt: Optional, RFC3339 format, cannot be in the past, max 10 years in future
// @Tags Content Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Param content body SwaggerStorageRequest true "Content to store"
// @Success 202 {object} SwaggerSuccessResponse "Content queued for storage"
// @Failure 400 {object} SwaggerErrorResponse "Validation failed or invalid request"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 413 {object} SwaggerErrorResponse "Content size exceeds MAX_CONTENT_SIZE limit"
// @Failure 415 {object} SwaggerErrorResponse "Content type not in ALLOWED_CONTENT_TYPES"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/content [post]
func (h *ContentHandler) StoreContent(c echo.Context) error {
	var req models.StorageRequest
	if err := c.Bind(&req); err != nil {
		h.logValidationError("Invalid request body", zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Invalid request body",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Perform comprehensive validation
	if err := h.validator.ValidateStorageRequest(&req); err != nil {
		h.logValidationError("Storage request validation failed",
			zap.String("id", req.ID), zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Validation failed",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Create content object (AccessCount now tracked separately)
	content := &models.Content{
		ID:          req.ID,
		Data:        req.Data,
		Type:        req.Type,
		Tag:         req.Tag,
		CreatedAt:   time.Now(),
		ExpiresAt:   req.ExpiresAt,
		AccessLimit: req.AccessLimit,
	}

	// Store content
	if err := h.storage.Store(content); err != nil {
		h.logError("Failed to store content", zap.String("id", req.ID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, models.StorageResponse{
			Success: false,
			Message: "Failed to store content",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Return HTTP 202 Accepted to indicate content is queued, not yet persisted
	return c.JSON(http.StatusAccepted, models.StorageResponse{
		Success: true,
		Message: "Content queued for storage",
		Data: map[string]string{
			"id":     req.ID,
			"status": "queued",
		},
	})
}

// GetContent handles GET /api/v1/content/{id}
// @Summary Retrieve content by ID with atomic access tracking
// @Description Retrieve specific content by ID with atomic access count increment and expiration checking
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **ENABLE_COMPRESSION**: Response may be compressed based on Accept-Encoding
// @Description
// @Description ## Atomic Access Tracking:
// @Description - Access count increment and expiration check are handled atomically in storage layer
// @Description - Prevents race conditions between access counting and expiration validation
// @Description - Access count is tracked separately from content data for performance
// @Description
// @Description ## Expiration Behavior:
// @Description - Returns 404 if content not found or time-expired
// @Description - Returns 410 if content has reached its access limit (access_count > access_limit)
// @Description - Time-based expiration: content expires after ExpiresAt timestamp
// @Description - Access-based expiration: content expires when access count exceeds AccessLimit
// @Description
// @Description ## ID Validation:
// @Description - Must be alphanumeric with dots, hyphens, underscores only
// @Description - No dangerous path patterns (no "..", leading/trailing dots)
// @Description - Maximum 255 characters, valid UTF-8
// @Tags Content Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Param id path string true "Content ID" example("user-123-document")
// @Success 200 {object} SwaggerSuccessResponse{data=SwaggerContent} "Content retrieved successfully"
// @Failure 400 {object} SwaggerErrorResponse "Invalid content ID format"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 404 {object} SwaggerErrorResponse "Content not found or expired"
// @Failure 410 {object} SwaggerErrorResponse "Content access limit reached"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/content/{id} [get]
func (h *ContentHandler) GetContent(c echo.Context) error {
	id := c.Param("id")

	// Validate the ID parameter
	if err := h.validator.ValidateID(id); err != nil {
		h.logValidationError("Invalid content ID in request",
			zap.String("id", id), zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Invalid content ID",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	content, err := h.storage.Get(id)
	if err != nil {
		if err == storage.ErrContentNotFound {
			return c.JSON(http.StatusNotFound, models.StorageResponse{
				Success: false,
				Message: "Content not found",
				Data:    map[string]string{"error": err.Error()},
			})
		}
		h.logError("Failed to retrieve content", zap.String("id", id), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, models.StorageResponse{
			Success: false,
			Message: "Failed to retrieve content",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Note: Access count increment and expiration check are now handled atomically in storage layer
	// Convert to swagger format for consistent API response
	swaggerContent := toSwaggerContentWithAccess(content)
	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Content retrieved successfully",
		Data:    swaggerContent,
	})
}

// DeleteContent handles DELETE /api/v1/content/{id}
// @Summary Delete content by ID with write conflict handling
// @Description Delete specific content by ID from the storage system with intelligent retry logic for write conflicts
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **ENABLE_WARN_LOGGING**: Controls retry attempt logging verbosity
// @Description
// @Description ## Write Conflict Handling:
// @Description - Automatically retries deletion if content is currently being written to queue
// @Description - Uses WaitForContentWrite to wait for ongoing write operations to complete
// @Description - Maximum 3 retry attempts with 100ms delay between attempts
// @Description - 5-second timeout per write completion wait
// @Description
// @Description ## Behavior:
// @Description - Permanently removes content from storage and cleans up access tracking
// @Description - Returns 404 if content not found
// @Description - Validates ID format before deletion (same rules as GET/POST)
// @Description - Handles race conditions with concurrent write operations gracefully
// @Tags Content Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Param id path string true "Content ID to delete" example("user-123-document")
// @Success 200 {object} SwaggerSuccessResponse "Content deleted successfully"
// @Failure 400 {object} SwaggerErrorResponse "Invalid content ID format"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 404 {object} SwaggerErrorResponse "Content not found"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/content/{id} [delete]
func (h *ContentHandler) DeleteContent(c echo.Context) error {
	id := c.Param("id")

	// Validate the ID parameter
	if err := h.validator.ValidateID(id); err != nil {
		h.logger.Warn("Invalid content ID in delete request", zap.String("id", id), zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Invalid content ID",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	if err := h.deleteWithRetry(id); err != nil {
		if err == storage.ErrContentNotFound {
			return c.JSON(http.StatusNotFound, models.StorageResponse{
				Success: false,
				Message: "Content not found",
				Data:    map[string]string{"error": err.Error()},
			})
		}

		// Log error (deleteWithRetry handles retry-specific logging internally)
		h.logError("Failed to delete content after retries", zap.String("id", id), zap.Error(err))

		return c.JSON(http.StatusInternalServerError, models.StorageResponse{
			Success: false,
			Message: "Failed to delete content",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Content deleted successfully",
		Data:    map[string]string{"id": id},
	})
}

// deleteWithRetry attempts to delete content with internal retry logic for write conflicts
// This handles the case where content is currently being written to the queue
func (h *ContentHandler) deleteWithRetry(id string) error {
	const (
		maxRetries     = 3
		retryTimeout   = 5 * time.Second
		retryDelay     = 100 * time.Millisecond
	)

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Attempt the delete operation
		err := h.storage.Delete(id)
		if err == nil {
			// Success - log if this was a retry
			if attempt > 0 {
				h.logger.Info("Delete succeeded after retry",
					zap.String("id", id),
					zap.Int("attempt", attempt+1))
			}
			return nil
		}

		// Check if this is a retryable "currently being written" error
		if strings.Contains(err.Error(), "currently being written") {
			if h.cfg.EnableWarnLogging {
				h.logger.Warn("Delete retry - content currently being written",
					zap.String("id", id),
					zap.Int("attempt", attempt+1),
					zap.Int("max_attempts", maxRetries))
			}

			// Wait for the write operation to complete
			waitErr := h.storage.WaitForContentWrite(id, retryTimeout)
			if waitErr != nil {
				// Write didn't complete in time - this becomes our error
				lastErr = waitErr
				h.logger.Warn("Write completion timeout during delete retry",
					zap.String("id", id),
					zap.Int("attempt", attempt+1),
					zap.Duration("timeout", retryTimeout),
					zap.Error(waitErr))
				break // Don't retry on timeout
			}

			// Write completed - add small delay before retry to ensure consistency
			if attempt < maxRetries-1 {
				time.Sleep(retryDelay)
			}
			lastErr = err
			continue
		}

		// Non-retryable error (e.g., content not found, database error)
		return err
	}

	// All retries exhausted or timeout occurred
	if lastErr != nil {
		return lastErr
	}

	// This shouldn't happen, but provide a fallback error
	return fmt.Errorf("delete failed after %d attempts", maxRetries)
}

// ListContent handles GET /api/v1/content with efficient pagination and filtering
// @Summary List content with advanced filtering and access count tracking
// @Description List all content with support for pagination, advanced filtering, and real-time access count information
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **ENABLE_COMPRESSION**: Response may be compressed based on Accept-Encoding
// @Description
// @Description ## Pagination:
// @Description - Default limit: 100, maximum: 1000 (hardcoded limit for performance)
// @Description - Use offset for pagination through large result sets
// @Description - Returns total count for pagination metadata
// @Description
// @Description ## Advanced Filtering:
// @Description - **type**: Filter by MIME type (e.g., "application/json", "text/plain")
// @Description - **tag**: Filter by content tag/classifier
// @Description - **created_after**: Filter content created after timestamp (RFC3339 format)
// @Description - **created_before**: Filter content created before timestamp (RFC3339 format)
// @Description - **include_expired**: Include expired content in results (default: false)
// @Description
// @Description ## Access Count Integration:
// @Description - Each content item includes real-time access_count from separate tracking system
// @Description - Access counts are retrieved efficiently for all returned items
// @Description - Expiration status is calculated using current access counts
// @Description
// @Description ## Filtering Options:
// @Description - **type**: Filter by content type (e.g., "text/plain")
// @Description - **tag**: Filter by content tag
// @Description - **created_after**: Filter content created after timestamp (RFC3339)
// @Description - **created_before**: Filter content created before timestamp (RFC3339)
// @Description - **include_expired**: Include expired content (default: false)
// @Tags Content Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Param limit query int false "Number of items per page (max: 1000)" default(100) example(50)
// @Param offset query int false "Number of items to skip" default(0) example(0)
// @Param type query string false "Filter by content type" example("text/plain")
// @Param tag query string false "Filter by content tag" example("user-documents")
// @Param created_after query string false "Filter content created after (RFC3339)" example("2024-01-01T00:00:00Z")
// @Param created_before query string false "Filter content created before (RFC3339)" example("2024-12-31T23:59:59Z")
// @Param include_expired query boolean false "Include expired content" default(false) example(false)
// @Success 200 {object} SwaggerContentListResponse "Content listed successfully"
// @Failure 400 {object} SwaggerErrorResponse "Invalid pagination or filter parameters"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/content [get]
func (h *ContentHandler) ListContent(c echo.Context) error {
	// Parse and validate query parameters for pagination
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	limit, offset, err := h.validator.ValidatePaginationParams(limitStr, offsetStr)
	if err != nil {
		h.logger.Warn("Invalid pagination parameters",
			zap.String("limit", limitStr),
			zap.String("offset", offsetStr),
			zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Invalid pagination parameters",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Parse filter parameters
	filterParams := make(map[string]string)
	for key, values := range c.QueryParams() {
		if len(values) > 0 {
			switch key {
			case "type", "tag", "created_after", "created_before", "include_expired":
				filterParams[key] = values[0]
			}
		}
	}

	filter, err := h.validator.ParseContentFilter(filterParams)
	if err != nil {
		h.logger.Warn("Invalid filter parameters", zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Invalid filter parameters",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Use efficient pagination with filtering
	contents, err := h.storage.ListWithFilter(limit, offset, filter)
	if err != nil {
		h.logger.Error("Failed to list content", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, models.StorageResponse{
			Success: false,
			Message: "Failed to list content",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Convert contents to swagger format with access counts
	swaggerContents := make([]SwaggerContent, len(contents))
	for i, content := range contents {
		// Get access count for each content item
		accessCount := h.storage.GetAccessCount(content.ID)
		contentWithAccess := &models.ContentWithAccess{
			Content:     content,
			AccessCount: accessCount,
		}
		swaggerContents[i] = toSwaggerContentWithAccess(contentWithAccess)
	}

	// Get total count with the same filter for pagination metadata
	total := h.storage.CountWithFilter(filter)

	response := map[string]interface{}{
		"contents": swaggerContents,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"filters":  filterParams, // Include applied filters in response for transparency
	}

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Content listed successfully",
		Data:    response,
	})
}

// GetContentCount handles GET /api/v1/content/count
// @Summary Get total content count with storage-level optimization
// @Description Get the total number of stored content items in the system using optimized storage-level counting
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **ENABLE_COMPRESSION**: Response may be compressed based on Accept-Encoding
// @Description
// @Description ## Performance Characteristics:
// @Description - Fast operation using storage-level counting (no iteration required)
// @Description - Returns total count of all content items (including expired and access-limited)
// @Description - Does not include queued items that haven't been persisted yet
// @Description - Consistent with database state, not affected by write queue contents
// @Tags Content Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Success 200 {object} SwaggerCountResponse "Content count retrieved successfully"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/content/count [get]
func (h *ContentHandler) GetContentCount(c echo.Context) error {
	count := h.storage.Count()
	response := map[string]int{"count": count}

	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Content count retrieved successfully",
		Data:    response,
	})
}

// GetContentStatus handles GET /api/v1/content/{id}/status
// @Summary Get content storage status with queue visibility
// @Description Check the current storage status of content with visibility into sequential write queue processing
// @Description
// @Description ## Configuration Dependencies:
// @Description - **ENABLE_AUTH**: Requires authentication when enabled
// @Description - **THROTTLE_LIMIT**: May return 429 if rate limit exceeded
// @Description - **Storage backend dependent**: Queue checking only available for BadgerDB storage
// @Description
// @Description ## Status Values and Processing Flow:
// @Description - **queued**: Content is in write queue awaiting sequential processing (HTTP 200)
// @Description - **stored**: Content is persisted to disk and available for retrieval (HTTP 200)
// @Description - **not_found**: Content ID does not exist in system or queue (HTTP 404)
// @Description
// @Description ## Use Cases:
// @Description - Monitor async content storage completion after POST /api/v1/content
// @Description - Verify content availability before attempting retrieval
// @Description - Debug storage pipeline issues and queue processing delays
// @Description - Implement polling-based confirmation of content persistence
// @Tags Content Operations
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Security ApiKeyQuery
// @Param id path string true "Content ID to check status" example("user-123-document")
// @Success 200 {object} SwaggerContentStatusResponse "Content status retrieved successfully"
// @Failure 400 {object} SwaggerErrorResponse "Invalid content ID format"
// @Failure 401 {object} SwaggerErrorResponse "Authentication required (when ENABLE_AUTH=true)"
// @Failure 404 {object} SwaggerErrorResponse "Content not found"
// @Failure 429 {object} SwaggerErrorResponse "Rate limit exceeded (THROTTLE_LIMIT)"
// @Failure 500 {object} SwaggerErrorResponse "Internal server error"
// @Router /api/v1/content/{id}/status [get]
func (h *ContentHandler) GetContentStatus(c echo.Context) error {
	id := c.Param("id")

	// Validate the ID parameter
	if err := h.validator.ValidateID(id); err != nil {
		h.logValidationError("Invalid content ID in status request",
			zap.String("id", id), zap.Error(err))
		return c.JSON(http.StatusBadRequest, models.StorageResponse{
			Success: false,
			Message: "Invalid content ID",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Check if content is in the write queue (pending)
	if badgerStorage, ok := h.storage.(*storage.BadgerStorage); ok {
		if badgerStorage.IsContentInQueue(id) {
			return c.JSON(http.StatusOK, models.StorageResponse{
				Success: true,
				Message: "Content status retrieved successfully",
				Data: map[string]string{
					"id":     id,
					"status": "queued",
				},
			})
		}
	}

	// Check if content exists in storage (persisted)
	_, err := h.storage.Get(id)
	if err != nil {
		if err == storage.ErrContentNotFound {
			return c.JSON(http.StatusNotFound, models.StorageResponse{
				Success: false,
				Message: "Content not found",
				Data: map[string]string{
					"id":     id,
					"status": "not_found",
				},
			})
		}
		h.logError("Failed to check content status", zap.String("id", id), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, models.StorageResponse{
			Success: false,
			Message: "Failed to check content status",
			Data:    map[string]string{"error": err.Error()},
		})
	}

	// Content exists and is persisted
	return c.JSON(http.StatusOK, models.StorageResponse{
		Success: true,
		Message: "Content status retrieved successfully",
		Data: map[string]string{
			"id":     id,
			"status": "stored",
		},
	})
}

// Legacy helper functions - no longer needed with Echo's built-in JSON responses
// Kept for reference during migration
