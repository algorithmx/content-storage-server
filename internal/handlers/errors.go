package handlers

import (
	"strings"
)

// sanitizeError returns a safe error message for clients
// Internal errors are sanitized to prevent information disclosure
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}

	// Check for known safe errors that can be exposed to clients
	errMsg := strings.ToLower(err.Error())
	safeErrors := map[string]string{
		"content not found":           "Content not found",
		"content has expired":         "Content has expired",
		"access limit exceeded":       "Access limit exceeded",
		"rate limit exceeded":         "Rate limit exceeded",
		"invalid api key":             "Invalid API key",
		"invalid id":                  "Invalid content ID",
		"invalid content type":        "Invalid content type",
		"content size exceeds":        "Content size exceeds maximum allowed",
		"unauthorized":                "Unauthorized",
		"forbidden":                   "Forbidden",
		"validation failed":           "Validation failed",
	}

	// Check if the error message contains any of our safe patterns
	for safePattern, userMessage := range safeErrors {
		if strings.Contains(errMsg, safePattern) {
			return userMessage
		}
	}

	// Partial-match fallback:
	// Some upstream error messages phrase "content not found" differently,
	// e.g. "The content was not found in storage" does not contain the
	// contiguous substring "content not found".
	//
	// We intentionally require both "content" and "not found" to avoid
	// misclassifying unrelated "file not found" or other "not found" errors.
	if strings.Contains(errMsg, "content") && strings.Contains(errMsg, "not found") {
		return "Content not found"
	}

	// Default: generic error message to prevent information disclosure
	return "An internal error occurred"
}

// sanitizeErrorWithPrefix returns a safe error message with a prefix
// Useful for adding context while still sanitizing the actual error
func sanitizeErrorWithPrefix(err error, prefix string) string {
	if err == nil {
		return ""
	}

	sanitized := sanitizeError(err)
	if sanitized == "An internal error occurred" {
		return prefix + ": " + sanitized
	}
	return sanitized
}
