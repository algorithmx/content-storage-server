package middleware

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimiter implements a high-performance throttling middleware with backlog queue support
// It limits concurrent requests and queues excess requests up to a configurable limit
// Optimized for minimal lock contention and efficient resource usage
type RateLimiter struct {
	maxConcurrent int64         // Maximum concurrent requests allowed
	maxBacklog    int           // Maximum requests that can be queued
	timeout       time.Duration // Timeout for queued requests
	backlog       chan struct{} // Buffered channel for backlog queue
	slots         chan struct{} // Semaphore for available slots
}

// NewRateLimiter creates a new rate limiter with the specified configuration
// Parameters:
//   - maxConcurrent: Maximum number of concurrent requests (THROTTLE_LIMIT)
//   - maxBacklog: Maximum number of queued requests (THROTTLE_BACKLOG_LIMIT)
//   - timeout: Timeout for queued requests (THROTTLE_BACKLOG_TIMEOUT)
func NewRateLimiter(maxConcurrent, maxBacklog int, timeout time.Duration) *RateLimiter {
	rl := &RateLimiter{
		maxConcurrent: int64(maxConcurrent),
		maxBacklog:    maxBacklog,
		timeout:       timeout,
		backlog:       make(chan struct{}, maxBacklog),
		slots:         make(chan struct{}, maxConcurrent),
	}

	// Pre-fill the semaphore with available slots
	for i := 0; i < maxConcurrent; i++ {
		rl.slots <- struct{}{}
	}

	return rl
}

// Middleware returns an Echo middleware function that implements high-performance throttling with backlog
// The middleware uses channel-based semaphores for optimal performance:
// 1. Try to acquire a slot immediately (non-blocking)
// 2. If no slots available, try to queue in backlog with timeout
// 3. If backlog full, reject with 429 Too Many Requests
func (rl *RateLimiter) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Stage 1: Try to acquire a slot immediately (fast path)
			select {
			case <-rl.slots:
				// Got a slot immediately, process request
				defer func() {
					rl.slots <- struct{}{} // Return slot to pool
				}()
				return next(c)
			default:
				// No slots available, try backlog
			}

			// Stage 2: Try to queue in backlog
			select {
			case rl.backlog <- struct{}{}:
				// Successfully queued, now wait for available slot with timeout
				defer func() { <-rl.backlog }() // Remove from backlog when done

				// Create timeout context for this request
				ctx, cancel := context.WithTimeout(c.Request().Context(), rl.timeout)
				defer cancel()

				// Wait for available slot with timeout
				select {
				case <-rl.slots:
					// Got a slot, process request
					defer func() {
						rl.slots <- struct{}{} // Return slot to pool
					}()
					return next(c)
				case <-ctx.Done():
					// Timeout exceeded
					return echo.NewHTTPError(429, "Request timeout in backlog")
				}
			default:
				// Stage 3: Backlog is full, reject immediately
				return echo.NewHTTPError(429, "Too many requests")
			}
		}
	}
}

// GetStats returns current statistics about the rate limiter
// Useful for monitoring and debugging - now lock-free for better performance
func (rl *RateLimiter) GetStats() map[string]interface{} {
	availableSlots := len(rl.slots)
	currentRequests := rl.maxConcurrent - int64(availableSlots) // Derived from semaphore state
	backlogLength := len(rl.backlog)

	return map[string]interface{}{
		"max_concurrent":   rl.maxConcurrent,
		"max_backlog":      rl.maxBacklog,
		"current_requests": currentRequests,
		"available_slots":  availableSlots,
		"backlog_length":   backlogLength,
		"timeout_duration": rl.timeout.String(),
	}
}
