package middleware

import (
	"content-storage-server/pkg/config"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

// SetupMiddleware configures the comprehensive HTTP middleware stack for enterprise-grade security and performance.
//
// This function applies a carefully ordered middleware stack that provides multi-layer security,
// performance optimization, and operational monitoring for the Content Storage Server.
//
// ## Middleware Application Order (Critical for Performance):
// 1. **Request ID** - Essential tracking for all requests (minimal overhead)
// 2. **Echo Rate Limiting** - Built-in rate limiter (first line of defense)
// 3. **Custom Rate Limiting** - Advanced throttling with backlog support
// 4. **IP Allowlisting** - Network-level security (early rejection of unauthorized IPs)
// 5. **Panic Recovery** - Graceful handling of application panics
// 6. **Security Headers** - HTTPS, XSS, and other security protections
// 7. **Compression** - Response compression for bandwidth optimization
// 8. **Request Timeout** - Prevents resource exhaustion from slow requests
// 9. **CORS** - Cross-origin resource sharing configuration
// 10. **Authentication** - API key validation (applied last for efficiency)
//
// ## Security Features:
// - **Multi-layer Rate Limiting**: Echo built-in + custom throttling with backlog
// - **IP Allowlisting**: Configurable network-level access control
// - **API Key Authentication**: Secure API access when enabled
// - **Security Headers**: Comprehensive HTTP security headers
// - **CORS Policy**: Configurable cross-origin access control
//
// ## Performance Optimizations:
// - **Early Rejection**: Rate limiting and IP filtering applied before expensive operations
// - **Conditional Middleware**: Only applies enabled features (compression, auth, etc.)
// - **Optimized Ordering**: Minimal overhead for high-traffic public endpoints
// - **Request Timeout**: Prevents resource exhaustion from slow clients
//
// ## Configuration Dependencies:
// - **ECHO_RATE_LIMIT**: Controls Echo built-in rate limiting
// - **THROTTLE_LIMIT**: Controls custom rate limiting with backlog
// - **ALLOWED_IPS**: Configures IP allowlisting (empty = disabled)
// - **ENABLE_COMPRESSION**: Controls response compression
// - **ENABLE_AUTH**: Controls API key authentication
// - **ALLOWED_ORIGINS**: Configures CORS policy
//
// Parameters:
// - e: Echo router instance to configure
// - cfg: Server configuration with middleware settings
// - appLogger: Structured logger for middleware operations
func SetupMiddleware(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger) {
	// Essential middleware before rate limiting (minimal overhead)
	e.Use(middleware.RequestID()) // Required for request tracking
	// Real IP extraction is handled automatically by Echo

	// ECHO RATE LIMITING - Built-in Echo rate limiter (first line of defense)
	if cfg.EchoRateLimit > 0 {
		e.Use(SetupEchoRateLimiter(cfg))
	}

	// CUSTOM RATE LIMITING - Custom throttle middleware with backlog support
	rateLimiter := NewRateLimiter(
		cfg.ThrottleLimit,
		cfg.ThrottleBacklogLimit,
		cfg.ThrottleBacklogTimeout,
	)
	e.Use(rateLimiter.Middleware())

	// IP ALLOWLIST - Network-level security (first line of defense)
	// This should be applied early to reject unauthorized IPs before expensive operations
	if len(cfg.AllowedIPs) > 0 {
		ipAllowlist := NewIPAllowlistMiddleware(cfg.AllowedIPs, appLogger)
		e.Use(ipAllowlist.Middleware())
	}

	// Remaining middleware applied only to accepted requests
	e.Use(middleware.Recover()) // Panic recovery for accepted requests

	// Security middleware
	e.Use(middleware.Secure())

	// Compression middleware (if enabled)
	if cfg.EnableCompression {
		e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
			Level: cfg.CompressionLevel,
		}))
	}

	// Request timeout
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: cfg.RequestTimeout,
	}))

	// CORS middleware
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-API-Key"},
		ExposeHeaders:    []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// API key authentication middleware (if enabled)
	if cfg.EnableAuth {
		e.Use(EchoAPIKeyMiddleware(cfg.APIKey, appLogger))
	}
}
