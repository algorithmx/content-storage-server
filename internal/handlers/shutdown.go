package handlers

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ShutdownHandler manages coordinated shutdown across HTTP and TLS servers
type ShutdownHandler struct {
	isShuttingDown int32      // Atomic flag for shutdown state
	logger         *zap.Logger // Logger for shutdown events
}

// NewShutdownHandler creates a new shutdown handler
func NewShutdownHandler(logger *zap.Logger) *ShutdownHandler {
	return &ShutdownHandler{
		isShuttingDown: 0,
		logger:         logger,
	}
}

// InitiateShutdown atomically sets the shutdown flag and logs the state change
func (sh *ShutdownHandler) InitiateShutdown() {
	if atomic.CompareAndSwapInt32(&sh.isShuttingDown, 0, 1) {
		sh.logger.Info("=== SHUTDOWN STATE ACTIVATED ===",
			zap.String("status", "All new incoming requests will be refused with HTTP 503"),
			zap.String("reason", "Server shutdown initiated"),
		)
		// Use fmt.Print for immediate console output during shutdown
		fmt.Println()
		fmt.Println("🚫 SHUTDOWN STATE: All new requests will be refused")
	}
}

// IsShuttingDown returns true if shutdown has been initiated
func (sh *ShutdownHandler) IsShuttingDown() bool {
	return atomic.LoadInt32(&sh.isShuttingDown) == 1
}

// Middleware returns Echo middleware that immediately refuses requests during shutdown
func (sh *ShutdownHandler) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check if server is shutting down
			if sh.IsShuttingDown() {
				// Return 503 Service Unavailable with clear message
				return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
					"error":     "Server is shutting down",
					"message":   "The server is currently shutting down and cannot accept new requests",
					"code":      "SERVER_SHUTTING_DOWN",
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				})
			}
			return next(c)
		}
	}
}

// GetShutdownStatePtr returns a pointer to the shutdown state for external packages
func (sh *ShutdownHandler) GetShutdownStatePtr() *int32 {
	return &sh.isShuttingDown
}
