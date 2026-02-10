package handlers

import (
	"errors"
	"strings"
	"testing"
)

// TestSanitizeErrorKnownSafeErrors tests that known safe errors are returned as-is
func TestSanitizeErrorKnownSafeErrors(t *testing.T) {
	safeErrors := map[string]string{
		"content not found":           "Content not found",
		"Content has expired":         "Content has expired",
		"Access limit exceeded":       "Access limit exceeded",
		"Rate limit exceeded":         "Rate limit exceeded",
		"Invalid API key":             "Invalid API key",
		"Invalid id":                  "Invalid content ID",
		"Invalid content type":        "Invalid content type",
		"Content size exceeds":        "Content size exceeds maximum allowed",
		"Unauthorized":                "Unauthorized",
		"Forbidden":                   "Forbidden",
		"Validation failed":           "Validation failed",
	}

	for errMsg, expectedMsg := range safeErrors {
		t.Run(errMsg, func(t *testing.T) {
			err := errors.New(errMsg)
			result := sanitizeError(err)
			if result != expectedMsg {
				t.Errorf("Expected %q, got %q", expectedMsg, result)
			}
		})
	}
}

// TestSanitizeErrorCaseInsensitive tests that error matching is case-insensitive
func TestSanitizeErrorCaseInsensitive(t *testing.T) {
	testCases := []struct {
		errorInput  string
		expectedMsg string
	}{
		{"Content not found", "Content not found"},
		{"CONTENT NOT FOUND", "Content not found"},
		{"content not found", "Content not found"},
		{"CoNtEnT nOt FoUnD", "Content not found"},
		{"This content has expired", "Content has expired"},
		{"Access limit exceeded for user", "Access limit exceeded"},
		{"Rate limit exceeded, try again later", "Rate limit exceeded"},
	}

	for _, tc := range testCases {
		t.Run(tc.errorInput, func(t *testing.T) {
			err := errors.New(tc.errorInput)
			result := sanitizeError(err)
			if result != tc.expectedMsg {
				t.Errorf("For error %q, expected %q, got %q", tc.errorInput, tc.expectedMsg, result)
			}
		})
	}
}

// TestSanitizeErrorInternalErrors tests that internal errors are sanitized
func TestSanitizeErrorInternalErrors(t *testing.T) {
	internalErrors := []string{
		"database connection failed",
		"file system error: permission denied",
		"panic: runtime error",
		"segmentation fault",
		"SQL injection detected in query",
		"/etc/passwd file not found",
		"internal server error: stack trace...",
		"error connecting to mysql database at 192.168.1.1",
	}

	for _, errMsg := range internalErrors {
		t.Run(errMsg, func(t *testing.T) {
			err := errors.New(errMsg)
			result := sanitizeError(err)
			if result != "An internal error occurred" {
				t.Errorf("Internal error should be sanitized, got %q", result)
			}
		})
	}
}

// TestSanitizeErrorNilError tests that nil error returns empty string
func TestSanitizeErrorNilError(t *testing.T) {
	result := sanitizeError(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil error, got %q", result)
	}
}

// TestSanitizeErrorWithPrefix tests prefix behavior
func TestSanitizeErrorWithPrefix(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		prefix         string
		expectedResult string
	}{
		{
			name:           "safe error without prefix",
			err:            errors.New("content not found"),
			prefix:         "",
			expectedResult: "Content not found",
		},
		{
			name:           "internal error with prefix",
			err:            errors.New("internal database error"),
			prefix:         "Failed to retrieve content",
			expectedResult: "Failed to retrieve content: An internal error occurred",
		},
		{
			name:           "safe error with prefix",
			err:            errors.New("content has expired"),
			prefix:         "Validation error",
			expectedResult: "Content has expired",
		},
		{
			name:           "nil error with prefix",
			err:            nil,
			prefix:         "Some prefix",
			expectedResult: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeErrorWithPrefix(tc.err, tc.prefix)
			if result != tc.expectedResult {
				t.Errorf("Expected %q, got %q", tc.expectedResult, result)
			}
		})
	}
}

// TestSanitizeErrorPreventsInformationDisclosure tests that sensitive info is not leaked
func TestSanitizeErrorPreventsInformationDisclosure(t *testing.T) {
	sensitiveErrors := []string{
		"Error: duplicate key value violates unique constraint \"users.email\"",
		"Query failed: SELECT * FROM users WHERE password = 'plain'",
		"Connection failed to postgresql://admin:password@localhost/db",
		"File not found: /var/www/.env with API_KEY=secret123",
		"Error: token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9 is invalid",
		"Exception: NullPointerException at com.example.Handler.process(Handler.java:123)",
		"Stack: main.process at /app/main.go:456",
	}

	for _, errMsg := range sensitiveErrors {
		t.Run(errMsg[:min(len(errMsg), 50)], func(t *testing.T) {
			err := errors.New(errMsg)
			result := sanitizeError(err)

			// Should sanitize to generic message
			if result != "An internal error occurred" {
				t.Errorf("Sensitive error was not sanitized, got: %q", result)
			}

			// Verify no sensitive keywords leaked
			sensitiveKeywords := []string{"password", "token", ".env", "localhost", "stack", "query", "postgres", "mysql"}
			lowerResult := strings.ToLower(result)
			for _, keyword := range sensitiveKeywords {
				if strings.Contains(lowerResult, keyword) {
					t.Errorf("Result contains sensitive keyword %q: %q", keyword, result)
				}
			}
		})
	}
}

// TestSanitizeErrorPartialMatchesWork tests that partial matches are recognized
func TestSanitizeErrorPartialMatchesWork(t *testing.T) {
	testCases := []struct {
		errorMsg    string
		expectedMsg string
	}{
		{
			"The content was not found in storage",
			"Content not found",
		},
		{
			"Request validation failed for field 'name'",
			"Validation failed",
		},
		{
			"User is unauthorized to access this resource",
			"Unauthorized",
		},
		{
			"Access forbidden: insufficient permissions",
			"Forbidden",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.errorMsg, func(t *testing.T) {
			err := errors.New(tc.errorMsg)
			result := sanitizeError(err)
			if result != tc.expectedMsg {
				t.Errorf("For %q, expected %q, got %q", tc.errorMsg, tc.expectedMsg, result)
			}
		})
	}
}

// TestSanitizeErrorWithComplexInternalErrors tests various internal error formats
func TestSanitizeErrorWithComplexInternalErrors(t *testing.T) {
	complexErrors := []struct {
		name string
		err  string
	}{
		{"wrapped error", "wrap: original error: connection refused"},
		{"formatted error", "error [code 500]: internal server error at /app/handler.go:123"},
		{"JSON error", `{"error":"internal","details":"database timeout","trace_id":"abc-123"}`},
		{"XML error", `<error>Internal error</error><trace>stack trace here</trace>`},
		{"path disclosure", "Error in /var/app/internal/secret_handler.go:45"},
		{"debug info", "[DEBUG] Error: nil pointer dereference at offset 0x1234"},
	}

	for _, tc := range complexErrors {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.err)
			result := sanitizeError(err)

			if result != "An internal error occurred" {
				t.Errorf("Complex internal error should be sanitized to generic message, got: %q", result)
			}
		})
	}
}

// TestSanitizeErrorDoesNotModifySafeErrorDetails tests that safe errors preserve their meaning
func TestSanitizeErrorDoesNotModifySafeErrorDetails(t *testing.T) {
	safeErrorsWithDetails := []struct {
		errorMsg    string
		expectedMsg string
	}{
		{"content not found: id abc123", "Content not found"},
		{"validation failed: field 'email' is required", "Validation failed"},
		{"rate limit exceeded: max 100 requests per minute", "Rate limit exceeded"},
	}

	for _, tc := range safeErrorsWithDetails {
		t.Run(tc.errorMsg, func(t *testing.T) {
			err := errors.New(tc.errorMsg)
			result := sanitizeError(err)
			if result != tc.expectedMsg {
				t.Errorf("Expected %q, got %q", tc.expectedMsg, result)
			}
		})
	}
}

// TestSanitizeErrorWithSpecialCharacters tests handling of special characters
func TestSanitizeErrorWithSpecialCharacters(t *testing.T) {
	specialCharErrors := []struct {
		name    string
		errMsg  string
		safeMsg string
	}{
		{"SQL injection", "' OR '1'='1", "An internal error occurred"},
		{"XSS attempt", "<script>alert('xss')</script>", "An internal error occurred"},
		{"Path traversal", "../../../etc/passwd", "An internal error occurred"},
		{"Shell command", "rm -rf /; echo hacked", "An internal error occurred"},
		{"Safe special chars", "Content not found! (retry later)", "Content not found"},
	}

	for _, tc := range specialCharErrors {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			result := sanitizeError(err)
			if result != tc.safeMsg {
				t.Errorf("Expected %q, got %q", tc.safeMsg, result)
			}

			// For unsafe inputs, verify sanitization
			if tc.safeMsg == "An internal error occurred" {
				// Check that raw input is not returned
				if strings.Contains(result, tc.errMsg) {
					t.Errorf("Result contains raw unsafe input: %q", result)
				}
			}
		})
	}
}

// TestSanitizeErrorWithVeryLongErrors tests handling of very long error messages
func TestSanitizeErrorWithVeryLongErrors(t *testing.T) {
	// Create a very long error message
	longErrorMsg := strings.Repeat("error ", 10000)
	err := errors.New(longErrorMsg)

	result := sanitizeError(err)

	// Should return generic message for unknown long errors
	if result != "An internal error occurred" {
		t.Errorf("Very long error should be sanitized, got: %q", result)
	}
}

// TestSanitizeErrorUnicode tests handling of unicode characters
func TestSanitizeErrorUnicode(t *testing.T) {
	unicodeErrors := []struct {
		name    string
		errMsg  string
		safeMsg string
	}{
		{"safe unicode", "Contenu non trouvé", "An internal error occurred"}, // French
		{"safe with emoji", "Content not found 🚫", "Content not found"},
		{"unsafe unicode", "Erro interno: banco de dados falhou", "An internal error occurred"}, // Portuguese
	}

	for _, tc := range unicodeErrors {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			result := sanitizeError(err)
			if result != tc.safeMsg {
				t.Errorf("Expected %q, got %q", tc.safeMsg, result)
			}
		})
	}
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
