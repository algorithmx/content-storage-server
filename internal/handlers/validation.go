package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"content-storage-server/pkg/config"
	"content-storage-server/pkg/models"
	"content-storage-server/pkg/storage"

	"github.com/go-playground/validator/v10"
)

// Validation constants and patterns
const (
	minAccessLimit = 0 // Minimum access limit (always 0)
)

var (
	// ID must be alphanumeric with hyphens, underscores, and dots
	validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	// Content type should follow MIME type pattern or simple type
	validTypePattern = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
)

// RequestValidator handles validation of incoming requests
type RequestValidator struct {
	config    *config.Config
	validator *validator.Validate
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(cfg *config.Config) *RequestValidator {
	v := validator.New()

	// Register custom validation for ID pattern
	v.RegisterValidation("alphanum_underscore_dash_dot", validateIDPattern)

	return &RequestValidator{
		config:    cfg,
		validator: v,
	}
}

// validateIDPattern validates that the field contains only alphanumeric characters, dots, hyphens, and underscores
func validateIDPattern(fl validator.FieldLevel) bool {
	return validIDPattern.MatchString(fl.Field().String())
}

// ValidateStorageRequest performs comprehensive validation on storage requests - OPTIMIZED
func (v *RequestValidator) ValidateStorageRequest(req *models.StorageRequest) error {
	// Performance optimization: Use high-performance validator for struct validation
	if v.config.PerformanceMode {
		// Fast path: Use go-playground/validator for basic struct validation
		if err := v.validator.Struct(req); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}

		// Only perform expensive validations if necessary
		// Quick data size check without full content validation
		if len(req.Data) > int(v.config.MaxContentSize) {
			return fmt.Errorf("data size (%d bytes) exceeds maximum allowed size (%d bytes)",
				len(req.Data), v.config.MaxContentSize)
		}

		// Quick expiration time check
		if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now()) {
			return fmt.Errorf("expiration time cannot be in the past")
		}

		return nil
	}

	// Conservative path: Full validation (for non-performance mode)
	// Validate ID
	if err := v.ValidateID(req.ID); err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	// Validate content type
	if err := v.ValidateContentType(req.Type); err != nil {
		return fmt.Errorf("invalid content type: %w", err)
	}

	// Validate tag if provided
	if req.Tag != "" {
		if err := v.ValidateTag(req.Tag); err != nil {
			return fmt.Errorf("invalid tag: %w", err)
		}
	}

	// Validate data size and content
	if err := v.ValidateData(req.Data, req.Type); err != nil {
		return fmt.Errorf("invalid data: %w", err)
	}

	// Validate access limit
	if err := v.ValidateAccessLimit(req.AccessLimit); err != nil {
		return fmt.Errorf("invalid access limit: %w", err)
	}

	// Validate expiration time
	if err := v.ValidateExpirationTime(req.ExpiresAt); err != nil {
		return fmt.Errorf("invalid expiration time: %w", err)
	}

	return nil
}

// ValidateID validates content ID format and length
func (v *RequestValidator) ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("ID is required")
	}

	if len(id) > v.config.MaxIDLength {
		return fmt.Errorf("ID too long (max %d characters)", v.config.MaxIDLength)
	}

	if !utf8.ValidString(id) {
		return fmt.Errorf("ID contains invalid UTF-8 characters")
	}

	if !validIDPattern.MatchString(id) {
		return fmt.Errorf("ID contains invalid characters (only alphanumeric, dots, hyphens, and underscores allowed)")
	}

	// Check for dangerous patterns
	if strings.Contains(id, "..") || strings.HasPrefix(id, ".") || strings.HasSuffix(id, ".") {
		return fmt.Errorf("ID contains dangerous path patterns")
	}

	return nil
}

// ValidateContentType validates content type format
func (v *RequestValidator) ValidateContentType(contentType string) error {
	if contentType == "" {
		return fmt.Errorf("content type is required")
	}

	if len(contentType) > v.config.MaxTypeLength {
		return fmt.Errorf("content type too long (max %d characters)", v.config.MaxTypeLength)
	}

	if !utf8.ValidString(contentType) {
		return fmt.Errorf("content type contains invalid UTF-8 characters")
	}

	if !validTypePattern.MatchString(contentType) {
		return fmt.Errorf("content type contains invalid characters")
	}

	return nil
}

// ValidateTag validates content tag format
func (v *RequestValidator) ValidateTag(tag string) error {
	// Empty tag is valid (optional field)
	if tag == "" {
		return nil
	}

	if len(tag) > v.config.MaxTypeLength { // Reuse the same length limit as content type
		return fmt.Errorf("tag too long (max %d characters)", v.config.MaxTypeLength)
	}

	if !utf8.ValidString(tag) {
		return fmt.Errorf("tag contains invalid UTF-8 characters")
	}

	// Allow alphanumeric characters, hyphens, underscores, and dots for tags
	if !validTypePattern.MatchString(tag) {
		return fmt.Errorf("tag contains invalid characters")
	}

	return nil
}

// ValidateData validates content data based on size and type
func (v *RequestValidator) ValidateData(data interface{}, contentType string) error {
	if data == nil {
		return fmt.Errorf("data is required")
	}

	// Check for empty string data
	if str, ok := data.(string); ok && str == "" {
		return fmt.Errorf("data cannot be empty")
	}

	// Calculate data size for validation
	dataSize, err := v.calculateDataSize(data)
	if err != nil {
		return fmt.Errorf("failed to calculate data size: %w", err)
	}

	// Check against configured maximum content size
	if dataSize > v.config.MaxContentSize {
		return fmt.Errorf("data size (%d bytes) exceeds maximum allowed size (%d bytes)",
			dataSize, v.config.MaxContentSize)
	}

	// Validate data based on content type
	if err := v.validateDataByType(data, contentType); err != nil {
		return fmt.Errorf("data validation failed: %w", err)
	}

	return nil
}

// calculateDataSize estimates the size of the data in bytes
func (v *RequestValidator) calculateDataSize(data interface{}) (int64, error) {
	switch d := data.(type) {
	case string:
		return int64(len(d)), nil
	case []byte:
		return int64(len(d)), nil
	case nil:
		return 0, nil
	default:
		// For other types, marshal to JSON to estimate size
		jsonData, err := json.Marshal(data)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal data for size calculation: %w", err)
		}
		return int64(len(jsonData)), nil
	}
}

// validateDataByType performs type-specific data validation
func (v *RequestValidator) validateDataByType(data interface{}, contentType string) error {
	switch {
	case strings.HasPrefix(contentType, "text/"):
		return v.validateTextData(data)
	case strings.HasPrefix(contentType, "application/json"):
		return v.validateJSONData(data)
	case strings.HasPrefix(contentType, "application/"):
		return v.validateBinaryData(data)
	default:
		// For unknown types, just ensure it's not nil
		if data == nil {
			return fmt.Errorf("data cannot be nil")
		}
	}
	return nil
}

// validateTextData validates text content
func (v *RequestValidator) validateTextData(data interface{}) error {
	str, ok := data.(string)
	if !ok {
		return fmt.Errorf("text content must be a string")
	}

	if !utf8.ValidString(str) {
		return fmt.Errorf("text content contains invalid UTF-8 characters")
	}

	return nil
}

// validateJSONData validates JSON content - OPTIMIZED for performance
func (v *RequestValidator) validateJSONData(data interface{}) error {
	// For JSON content type, the data should be a valid JSON string
	str, ok := data.(string)
	if !ok {
		return fmt.Errorf("JSON content must be a string")
	}

	// Validate that it's valid JSON by attempting to unmarshal
	var jsonData interface{}
	if err := json.Unmarshal([]byte(str), &jsonData); err != nil {
		return fmt.Errorf("invalid JSON format: %w", err)
	}

	return nil
}

// validateBinaryData validates binary content
func (v *RequestValidator) validateBinaryData(data interface{}) error {
	switch data.(type) {
	case []byte, string:
		return nil
	default:
		// For other types, ensure they can be marshaled
		if _, err := json.Marshal(data); err != nil {
			return fmt.Errorf("binary data must be serializable: %w", err)
		}
	}
	return nil
}

// ValidateAccessLimit validates access limit bounds
func (v *RequestValidator) ValidateAccessLimit(accessLimit int) error {
	if accessLimit < minAccessLimit {
		return fmt.Errorf("access limit cannot be negative")
	}

	if accessLimit > v.config.MaxAccessLimit {
		return fmt.Errorf("access limit too high (max %d)", v.config.MaxAccessLimit)
	}

	return nil
}

// ValidateExpirationTime validates expiration time
func (v *RequestValidator) ValidateExpirationTime(expiresAt *time.Time) error {
	if expiresAt == nil {
		return nil // Optional field
	}

	// Check if expiration time is in the past
	if expiresAt.Before(time.Now()) {
		return fmt.Errorf("expiration time cannot be in the past")
	}

	// Check if expiration time is too far in the future (e.g., more than 10 years)
	maxFutureTime := time.Now().AddDate(10, 0, 0)
	if expiresAt.After(maxFutureTime) {
		return fmt.Errorf("expiration time too far in the future (max 10 years)")
	}

	return nil
}

// ValidatePaginationParams validates pagination parameters
func (v *RequestValidator) ValidatePaginationParams(limitStr, offsetStr string) (int, int, error) {
	limit := 100 // default limit
	offset := 0  // default offset

	// Validate limit parameter
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid limit parameter: must be a number")
		}
		if parsedLimit <= 0 {
			return 0, 0, fmt.Errorf("invalid limit parameter: must be positive")
		}
		if parsedLimit > v.config.MaxPaginationLimit {
			return 0, 0, fmt.Errorf("invalid limit parameter: maximum allowed is %d", v.config.MaxPaginationLimit)
		}
		limit = parsedLimit
	}

	// Validate offset parameter
	if offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid offset parameter: must be a number")
		}
		if parsedOffset < 0 {
			return 0, 0, fmt.Errorf("invalid offset parameter: cannot be negative")
		}
		offset = parsedOffset
	}

	return limit, offset, nil
}

// ParseContentFilter parses query parameters into a ContentFilter
func (v *RequestValidator) ParseContentFilter(params map[string]string) (*storage.ContentFilter, error) {
	filter := &storage.ContentFilter{}

	// Parse content type filter
	if contentType := params["type"]; contentType != "" {
		// Basic MIME type validation
		if !strings.Contains(contentType, "/") {
			return nil, fmt.Errorf("invalid content type format: must be in format 'type/subtype'")
		}
		filter.ContentType = contentType
	}

	// Parse tag filter
	if tag := params["tag"]; tag != "" {
		if err := v.ValidateTag(tag); err != nil {
			return nil, fmt.Errorf("invalid tag filter: %w", err)
		}
		filter.Tag = tag
	}

	// Parse created_after filter
	if createdAfter := params["created_after"]; createdAfter != "" {
		t, err := time.Parse(time.RFC3339, createdAfter)
		if err != nil {
			return nil, fmt.Errorf("invalid created_after format: must be RFC3339 format (e.g., 2023-01-01T00:00:00Z)")
		}
		filter.CreatedAfter = &t
	}

	// Parse created_before filter
	if createdBefore := params["created_before"]; createdBefore != "" {
		t, err := time.Parse(time.RFC3339, createdBefore)
		if err != nil {
			return nil, fmt.Errorf("invalid created_before format: must be RFC3339 format (e.g., 2023-01-01T00:00:00Z)")
		}
		filter.CreatedBefore = &t
	}

	// Parse include_expired filter
	if includeExpired := params["include_expired"]; includeExpired != "" {
		switch strings.ToLower(includeExpired) {
		case "true", "1", "yes":
			filter.IncludeExpired = true
		case "false", "0", "no":
			filter.IncludeExpired = false
		default:
			return nil, fmt.Errorf("invalid include_expired value: must be true/false, 1/0, or yes/no")
		}
	}

	// Validate date range if both are provided
	if filter.CreatedAfter != nil && filter.CreatedBefore != nil {
		if filter.CreatedAfter.After(*filter.CreatedBefore) {
			return nil, fmt.Errorf("invalid date range: created_after must be before created_before")
		}
	}

	return filter, nil
}
