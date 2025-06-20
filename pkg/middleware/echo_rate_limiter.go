package middleware

import (
	"content-storage-server/pkg/config"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// SetupEchoRateLimiter configures the built-in Echo rate limiter middleware
// This provides IP-based rate limiting as the first line of defense
func SetupEchoRateLimiter(cfg *config.Config) echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(cfg.EchoRateLimit),
				Burst:     cfg.EchoBurstLimit,
				ExpiresIn: cfg.EchoRateLimitExpiresIn,
			},
		),
		IdentifierExtractor: func(ctx echo.Context) (string, error) {
			id := ctx.RealIP()
			return id, nil
		},
		ErrorHandler: func(context echo.Context, err error) error {
			return context.JSON(429, map[string]string{"error": "rate limit exceeded"})
		},
		DenyHandler: func(context echo.Context, identifier string, err error) error {
			return context.JSON(429, map[string]string{"error": "too many requests"})
		},
	})
}
