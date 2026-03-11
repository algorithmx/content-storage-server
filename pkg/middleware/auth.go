package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"content-storage-server/pkg/config"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// EchoAPIKeyMiddleware provides API key authentication for Echo
// It skips authentication for health checks, heartbeat, and profiler endpoints
func EchoAPIKeyMiddleware(cfg *config.Config, appLogger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path

			// Always public endpoints (health checks, heartbeat, profiler)
			if path == "/health" || path == "/health/detailed" || path == "/ping" || strings.HasPrefix(path, "/debug/") {
				return next(c)
			}

			// Conditionally public endpoints based on config
			// Management UI - requires auth if RequireAuthForUI is true
			if !cfg.RequireAuthForUI && (path == "/" || strings.HasPrefix(path, "/static/")) {
				return next(c)
			}
			// Swagger documentation - requires auth if EnableSwagger is false
			if cfg.EnableSwagger && strings.HasPrefix(path, "/swagger/") {
				return next(c)
			}

			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey == "" && cfg.AllowQueryParamAuth {
				// Only allow query parameter auth if explicitly enabled (development only)
				apiKey = c.QueryParam("api_key")
			}

			// Use constant-time comparison to prevent timing attacks
			// Both strings must be non-empty for secure comparison
			if len(apiKey) == 0 || len(cfg.APIKey) == 0 ||
				subtle.ConstantTimeCompare([]byte(apiKey), []byte(cfg.APIKey)) != 1 {
				appLogger.Warn("Unauthorized API access attempt",
					zap.String("ip", c.RealIP()),
					zap.String("path", path),
					zap.String("user_agent", c.Request().UserAgent()),
					zap.String("method", c.Request().Method))
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid API key")
			}

			return next(c)
		}
	}
}

