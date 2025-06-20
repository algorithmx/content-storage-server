package middleware

import (
	"net"
	"net/http"
	"sort"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// ipCache provides high-performance caching for IP allowlist decisions using HashiCorp's LRU
// This significantly reduces the overhead of repeated IP parsing and network checks
type ipCache struct {
	cache *lru.Cache[string, bool] // Thread-safe LRU cache from HashiCorp
}

// newIPCache creates a new IP cache with LRU eviction using HashiCorp's mature library
func newIPCache(maxSize int) *ipCache {
	cache, err := lru.New[string, bool](maxSize)
	if err != nil {
		// Fallback to smaller cache if creation fails
		cache, _ = lru.New[string, bool](100)
	}
	return &ipCache{
		cache: cache,
	}
}

// get retrieves the cached decision for an IP address (thread-safe, lock-free reads)
func (c *ipCache) get(ip string) (bool, bool) {
	return c.cache.Get(ip)
}

// set stores the decision for an IP address with automatic LRU eviction
func (c *ipCache) set(ip string, allowed bool) {
	c.cache.Add(ip, allowed)
}

// IPAllowlistMiddleware provides IP-based access control for Echo
// It checks the client IP against a list of allowed IPs/CIDR blocks
// This provides network-level security as the first line of defense
type IPAllowlistMiddleware struct {
	allowedIPs      []string        // List of allowed IP addresses and CIDR blocks
	exactIPs        map[string]bool // Exact IP matches for O(1) lookup
	allowedNetworks []*net.IPNet    // Parsed CIDR networks sorted by specificity
	appLogger       *zap.Logger     // Logger for security events
	cache           *ipCache        // High-performance IP decision cache with LRU eviction
	healthPaths     map[string]bool // Pre-compiled health endpoint paths for fast lookup
}

// NewIPAllowlistMiddleware creates a new IP allowlist middleware with optimized parsing
// Parameters:
//   - allowedIPs: List of allowed IP addresses and CIDR blocks (e.g., ["192.168.1.0/24", "10.0.0.5"])
//   - appLogger: Logger for security events
func NewIPAllowlistMiddleware(allowedIPs []string, appLogger *zap.Logger) *IPAllowlistMiddleware {
	middleware := &IPAllowlistMiddleware{
		allowedIPs:      allowedIPs,
		exactIPs:        make(map[string]bool),
		allowedNetworks: make([]*net.IPNet, 0),
		appLogger:       appLogger,
		cache:           newIPCache(1000), // Cache up to 1000 IP decisions with LRU eviction
		healthPaths: map[string]bool{
			"/health":          true,
			"/health/detailed": true,
			"/ping":            true,
		},
	}

	// Parse IP addresses and CIDR blocks with optimized approach
	for _, ipStr := range allowedIPs {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Check if it's a CIDR block
		if strings.Contains(ipStr, "/") {
			_, network, err := net.ParseCIDR(ipStr)
			if err != nil {
				continue
			}
			middleware.allowedNetworks = append(middleware.allowedNetworks, network)
		} else {
			// Single IP address - add to exact match map for O(1) lookup
			ip := net.ParseIP(ipStr)
			if ip != nil {
				// Store normalized IP string for exact matching (avoids double parsing)
				middleware.exactIPs[ip.String()] = true
			}
		}
	}

	// Sort networks by specificity (most specific first) for faster matching
	// More specific networks (larger masks) are checked first
	sort.Slice(middleware.allowedNetworks, func(i, j int) bool {
		maskI, _ := middleware.allowedNetworks[i].Mask.Size()
		maskJ, _ := middleware.allowedNetworks[j].Mask.Size()
		return maskI > maskJ // Larger mask = more specific
	})

	return middleware
}

// Middleware returns an Echo middleware function that enforces IP allowlist
// If the allowlist is empty, all IPs are allowed (no restrictions)
// This middleware should be applied early in the middleware chain for optimal security
func (ipm *IPAllowlistMiddleware) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// If no IP restrictions are configured, allow all
			if len(ipm.allowedNetworks) == 0 && len(ipm.exactIPs) == 0 {
				return next(c)
			}

			// Fast path: Skip IP check for health endpoints using pre-compiled map
			path := c.Request().URL.Path
			if ipm.healthPaths[path] {
				return next(c)
			}

			// Get client IP (Echo automatically handles X-Forwarded-For, X-Real-IP, etc.)
			clientIP := c.RealIP()

			// Fast path: Check cache first to avoid expensive operations
			if allowed, exists := ipm.cache.get(clientIP); exists {
				if !allowed {
					// Don't log cached denials to reduce overhead
					return echo.NewHTTPError(http.StatusForbidden, "Access denied: IP not allowed")
				}
				return next(c)
			}

			// Slow path: Parse IP and check against networks (cache miss)
			ip := net.ParseIP(clientIP)
			if ip == nil {
				// Cache invalid IP format as denied
				ipm.cache.set(clientIP, false)
				return echo.NewHTTPError(http.StatusForbidden, "Invalid IP format")
			}

			// Fast exact IP match check (O(1) lookup)
			allowed := ipm.exactIPs[ip.String()]

			// If not an exact match, check against CIDR networks (sorted by specificity)
			if !allowed {
				for _, network := range ipm.allowedNetworks {
					if network.Contains(ip) {
						allowed = true
						break
					}
				}
			}

			// Cache the decision for future requests
			ipm.cache.set(clientIP, allowed)

			if !allowed {
				// Only log first-time denials to reduce overhead
				ipm.appLogger.Warn("IP access denied - not in allowlist",
					zap.String("client_ip", clientIP),
					zap.String("path", path))
				return echo.NewHTTPError(http.StatusForbidden, "Access denied: IP not allowed")
			}

			// IP is allowed, proceed to next middleware
			return next(c)
		}
	}
}

// IsIPAllowed checks if a given IP address is allowed by the current configuration
// This method now uses caching for improved performance
func (ipm *IPAllowlistMiddleware) IsIPAllowed(ipStr string) bool {
	// If no restrictions, all IPs are allowed
	if len(ipm.allowedNetworks) == 0 && len(ipm.exactIPs) == 0 {
		return true
	}

	// Check cache first for performance
	if allowed, exists := ipm.cache.get(ipStr); exists {
		return allowed
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		// Cache invalid IP as denied
		ipm.cache.set(ipStr, false)
		return false
	}

	// Fast exact IP match check
	allowed := ipm.exactIPs[ip.String()]

	// If not exact match, check networks (sorted by specificity)
	if !allowed {
		for _, network := range ipm.allowedNetworks {
			if network.Contains(ip) {
				allowed = true
				break
			}
		}
	}

	// Cache the result for future calls
	ipm.cache.set(ipStr, allowed)
	return allowed
}

// GetAllowedNetworks returns the list of allowed networks for debugging
// This includes both exact IPs (as CIDR) and actual CIDR networks
func (ipm *IPAllowlistMiddleware) GetAllowedNetworks() []string {
	var networks []string

	// Add exact IPs as CIDR networks
	for ipStr := range ipm.exactIPs {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			if ip.To4() != nil {
				networks = append(networks, ipStr+"/32") // IPv4
			} else {
				networks = append(networks, ipStr+"/128") // IPv6
			}
		}
	}

	// Add actual CIDR networks
	for _, network := range ipm.allowedNetworks {
		networks = append(networks, network.String())
	}

	return networks
}

// GetCacheStats returns cache statistics for monitoring and debugging
func (ipm *IPAllowlistMiddleware) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cache_size":     ipm.cache.cache.Len(),
		"cache_max_size": 1000, // Fixed size from newIPCache
		"cache_usage":    float64(ipm.cache.cache.Len()) / 1000.0 * 100,
	}
}

// ClearCache clears the IP decision cache (useful for testing or configuration changes)
func (ipm *IPAllowlistMiddleware) ClearCache() {
	ipm.cache.cache.Purge()
}
