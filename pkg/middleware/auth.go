package middleware

import (
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
			// Skip authentication for health checks, heartbeat, profiler endpoints, and static files
			path := c.Request().URL.Path
			if path == "/health" || path == "/health/detailed" ||
				path == "/ping" || strings.HasPrefix(path, "/debug/") ||
				strings.HasPrefix(path, "/static/") || path == "/" ||
				strings.HasPrefix(path, "/swagger/") {
				return next(c)
			}

			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey == "" && cfg.AllowQueryParamAuth {
				// Only allow query parameter auth if explicitly enabled (development only)
				apiKey = c.QueryParam("api_key")
			}

			if apiKey != cfg.APIKey {
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

