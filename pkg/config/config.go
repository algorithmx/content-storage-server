package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the content storage server
type Config struct {
	// =============================================================================
	// GROUP 1: HTTP SERVER SETTINGS
	// =============================================================================
	Port         string        // HTTP server port
	Host         string        // HTTP server host/bind address
	ReadTimeout  time.Duration // HTTP read timeout
	WriteTimeout time.Duration // HTTP write timeout

	// =============================================================================
	// GROUP 1.1: TLS/HTTPS SETTINGS
	// =============================================================================
	EnableTLS       bool     // Enable HTTPS with automatic Let's Encrypt certificates
	TLSPort         string   // HTTPS server port (default: 443)
	TLSCacheDir     string   // Directory to cache TLS certificates
	TLSHosts        []string // Allowed hostnames for TLS certificates (required for production)
	EnableHTTPSOnly bool     // Redirect all HTTP traffic to HTTPS

	// =============================================================================
	// GROUP 2: STORAGE SYSTEM SETTINGS
	// =============================================================================
	DataDir   string // Directory for BadgerDB data files
	BackupDir string // Directory for backup files

	// =============================================================================
	// GROUP 3: STORAGE RELIABILITY SETTINGS
	// =============================================================================
	BackupInterval   time.Duration // How often to create backups
	MaxBackups       int           // Maximum number of backups to retain
	EnableAdaptiveGC bool          // Enable adaptive garbage collection
	GCInterval       time.Duration // Garbage collection interval
	GCThreshold      float64       // Minimum ratio of reclaimable space to trigger GC (0.0-1.0)

	// Emergency recovery settings
	EnableEmergencyRecovery bool // Enable automatic recovery from emergency shutdown files on startup

	// =============================================================================
	// GROUP 4: STORAGE PERFORMANCE SETTINGS
	// =============================================================================
	PerformanceMode   bool  // Enable BadgerDB performance optimizations
	CacheSize         int64 // In-memory cache size in bytes for BadgerDB
	EnableCacheWarmup bool  // Enable cache warming on startup for consistent performance

	// Write queue optimization settings (for asynchronous writes)
	// Note: Uses single writer to ensure sequential ordering of operations
	WriteQueueSize         int           // Size of the write queue buffer
	WriteQueueBatchSize    int           // Number of writes to batch together
	WriteQueueBatchTimeout time.Duration // Maximum time to wait before flushing a batch

	// =============================================================================
	// GROUP 5: AUTHENTICATION & AUTHORIZATION SETTINGS
	// =============================================================================
	APIKey         string   // API key for authentication
	EnableAuth     bool     // Enable API key authentication
	AllowedOrigins []string // CORS allowed origins
	AllowedIPs     []string // IP allowlist for network-level security

	// =============================================================================
	// GROUP 6: VALIDATION SETTINGS
	// =============================================================================
	MaxIDLength        int // Maximum length for content IDs
	MaxTypeLength      int // Maximum length for content types
	MaxPaginationLimit int // Maximum pagination limit
	MaxAccessLimit     int // Maximum access limit for content

	// =============================================================================
	// GROUP 7: REQUEST PROCESSING SETTINGS
	// =============================================================================
	MaxContentSize  int64         // Maximum content size in bytes
	RequestTimeout  time.Duration // Timeout for individual requests
	ShutdownTimeout time.Duration // Graceful shutdown timeout

	// =============================================================================
	// GROUP 8: RATE LIMITING SETTINGS
	// =============================================================================
	// NOTE: ThrottleBacklog middleware temporarily disabled for Echo migration (Stage 1)
	// Will be re-enabled with Echo-compatible implementation in Stage 3
	ThrottleLimit          int           // Maximum concurrent requests
	ThrottleBacklogLimit   int           // Maximum queued requests
	ThrottleBacklogTimeout time.Duration // Timeout for queued requests

	// Echo-specific rate limiting
	APIRateLimitPeriod     time.Duration `env:"API_RATE_LIMIT_PERIOD" envDefault:"1h"`
	APIRateLimitBurst      int           `env:"API_RATE_LIMIT_BURST" envDefault:"10"`
	EchoRateLimit          float64       `env:"ECHO_RATE_LIMIT" envDefault:"100"`
	EchoBurstLimit         int           `env:"ECHO_BURST_LIMIT" envDefault:"200"`
	EchoRateLimitExpiresIn time.Duration `env:"ECHO_RATE_LIMIT_EXPIRES_IN" envDefault:"3m"`

	// =============================================================================
	// GROUP 8: HTTP COMPRESSION SETTINGS
	// =============================================================================
	EnableCompression bool     // Enable response compression
	CompressionLevel  int      // Compression level (1-9)
	CompressionTypes  []string // Content types to compress

	// =============================================================================
	// GROUP 9: SECURITY & DEBUGGING SETTINGS
	// =============================================================================
	EnableProfiler      bool              // Enable pprof endpoints
	AllowedContentTypes []string          // Allowed request content types
	SecurityHeaders     map[string]string // Custom security headers

	// =============================================================================
	// GROUP 10: LOGGING SETTINGS
	// =============================================================================
	// Selective logging settings
	EnableRequestLogging    bool `env:"ENABLE_REQUEST_LOGGING"`
	EnableManagementLogging bool `env:"ENABLE_MANAGEMENT_LOGGING"`
	EnableSecurityLogging   bool `env:"ENABLE_SECURITY_LOGGING"`
	EnableCriticalLogging   bool `env:"ENABLE_CRITICAL_LOGGING"`
	SuppressCRUDLogging     bool `env:"SUPPRESS_CRUD_LOGGING"`

	// Performance-oriented logging controls
	EnableErrorLogging      bool `env:"ENABLE_ERROR_LOGGING"`      // Control error-level logging
	EnableWarnLogging       bool `env:"ENABLE_WARN_LOGGING"`       // Control warning-level logging
	EnableValidationLogging bool `env:"ENABLE_VALIDATION_LOGGING"` // Control validation error logging
}

// Load loads configuration from environment variables
func Load() *Config {
	// Attempt to load .env file but proceed if not found
	godotenv.Load()

	config := &Config{
		// Server settings
		Port:         env("PORT", "8081"),
		Host:         env("HOST", "0.0.0.0"),
		ReadTimeout:  envDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout: envDuration("WRITE_TIMEOUT", 30*time.Second),

		// TLS/HTTPS settings
		EnableTLS:       envBool("ENABLE_TLS", false),
		TLSPort:         env("TLS_PORT", "443"),
		TLSCacheDir:     env("TLS_CACHE_DIR", "./certs"),
		TLSHosts:        envStringSlice("TLS_HOSTS", []string{}), // Empty means allow any host (development only)
		EnableHTTPSOnly: envBool("ENABLE_HTTPS_ONLY", false),

		// Storage settings
		DataDir:   env("DATA_DIR", "./data"),
		BackupDir: env("BACKUP_DIR", "./backups"),

		// Enhanced storage reliability settings
		BackupInterval:   envDuration("BACKUP_INTERVAL", 6*time.Hour),
		MaxBackups:       envInt("MAX_BACKUPS", 7),
		EnableAdaptiveGC: envBool("ENABLE_ADAPTIVE_GC", false),        // Disable adaptive GC for consistent performance
		GCInterval:       envDuration("GC_INTERVAL", 5*time.Minute),   // More frequent GC for better performance
		GCThreshold:      envFloat64("GC_THRESHOLD", 0.3),             // Lower threshold for more aggressive cleanup

		// Emergency recovery settings
		EnableEmergencyRecovery: envBool("ENABLE_EMERGENCY_RECOVERY", false), // Disabled by default for safety

		// Performance optimization settings
		PerformanceMode:   envBool("PERFORMANCE_MODE", true),   // Enable performance mode by default
		CacheSize:         envInt64("CACHE_SIZE", 128<<20),     // Default 128MB cache
		EnableCacheWarmup: envBool("ENABLE_CACHE_WARMUP", true), // Enable cache warmup by default

		// Write queue optimization settings (for asynchronous writes)
		// Note: Uses single writer to ensure sequential ordering of operations
		WriteQueueSize:         envInt("WRITE_QUEUE_SIZE", 10000),                         // Default queue size for high throughput
		WriteQueueBatchSize:    envInt("WRITE_QUEUE_BATCH_SIZE", 100),                    // Default batch size
		WriteQueueBatchTimeout: envDuration("WRITE_QUEUE_BATCH_TIMEOUT", 5*time.Millisecond), // Optimized batch timeout for better throughput

		// Validation settings
		MaxIDLength:        envInt("MAX_ID_LENGTH", 255),        // Maximum ID length
		MaxTypeLength:      envInt("MAX_TYPE_LENGTH", 100),      // Maximum content type length
		MaxPaginationLimit: envInt("MAX_PAGINATION_LIMIT", 1000), // Maximum pagination limit
		MaxAccessLimit:     envInt("MAX_ACCESS_LIMIT", 1000000), // Maximum access limit

		// Security settings
		APIKey:         env("API_KEY", ""),
		EnableAuth:     envBool("ENABLE_AUTH", false),
		AllowedOrigins: envStringSlice("ALLOWED_ORIGINS", []string{"*"}),
		AllowedIPs:     envStringSlice("ALLOWED_IPS", []string{}), // Empty means no IP restrictions

		// Performance settings
		MaxContentSize:  envInt64("MAX_CONTENT_SIZE", 10*1024*1024), // 10MB default
		RequestTimeout:  envDuration("REQUEST_TIMEOUT", 30*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 30*time.Second),

		// Rate limiting settings
		ThrottleLimit:          envInt("THROTTLE_LIMIT", 1000),
		ThrottleBacklogLimit:   envInt("THROTTLE_BACKLOG_LIMIT", 50),
		ThrottleBacklogTimeout: envDuration("THROTTLE_BACKLOG_TIMEOUT", 30*time.Second),

		// Compression settings
		EnableCompression: envBool("ENABLE_COMPRESSION", true),
		CompressionLevel:  envInt("COMPRESSION_LEVEL", 5),
		CompressionTypes: envStringSlice("COMPRESSION_TYPES", []string{
			"text/html", "text/css", "text/javascript", "application/javascript",
			"application/json", "application/xml", "text/xml", "text/plain",
		}),

		// Security settings
		EnableProfiler:      envBool("ENABLE_PROFILER", false),
		AllowedContentTypes: envStringSlice("ALLOWED_CONTENT_TYPES", []string{"application/json", "text/plain"}),
		SecurityHeaders: envStringMap("SECURITY_HEADERS", map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"X-XSS-Protection":       "1; mode=block",
		}),

		// Selective logging settings
		EnableRequestLogging:    envBool("ENABLE_REQUEST_LOGGING", false),   // Legacy request logging
		EnableManagementLogging: envBool("ENABLE_MANAGEMENT_LOGGING", true), // Management operations
		EnableSecurityLogging:   envBool("ENABLE_SECURITY_LOGGING", true),   // Security events
		EnableCriticalLogging:   envBool("ENABLE_CRITICAL_LOGGING", true),   // Critical system events
		SuppressCRUDLogging:     envBool("SUPPRESS_CRUD_LOGGING", true),     // Suppress CRUD logs

		// Performance-oriented logging controls
		EnableErrorLogging:      envBool("ENABLE_ERROR_LOGGING", true),      // Error logging (default: enabled)
		EnableWarnLogging:       envBool("ENABLE_WARN_LOGGING", true),       // Warning logging (default: enabled)
		EnableValidationLogging: envBool("ENABLE_VALIDATION_LOGGING", false), // Validation logging (default: disabled for performance)

		APIRateLimitPeriod:     envDuration("API_RATE_LIMIT_PERIOD", 1*time.Hour),
		APIRateLimitBurst:      envInt("API_RATE_LIMIT_BURST", 10),
		EchoRateLimit:          envFloat64("ECHO_RATE_LIMIT", 100),
		EchoBurstLimit:         envInt("ECHO_BURST_LIMIT", 200),
		EchoRateLimitExpiresIn: envDuration("ECHO_RATE_LIMIT_EXPIRES_IN", 3*time.Minute),
	}

	return config
}

// displayConfiguration shows the current configuration
func (cfg *Config) DisplayConfiguration() {
	fmt.Println("⚙️  Configuration:")
	fmt.Printf("   Port: %s\n", cfg.Port)
	fmt.Printf("   Host: %s\n", cfg.Host)
	if cfg.EnableTLS {
		fmt.Printf("   TLS Port: %s\n", cfg.TLSPort)
		fmt.Printf("   TLS Cache Dir: %s\n", cfg.TLSCacheDir)
		if len(cfg.TLSHosts) > 0 {
			fmt.Printf("   TLS Hosts: %v\n", cfg.TLSHosts)
		} else {
			fmt.Printf("   TLS Hosts: Any (development mode)\n")
		}
		fmt.Printf("   HTTPS Only: %t\n", cfg.EnableHTTPSOnly)
	}
	fmt.Printf("   Data Directory: %s\n", cfg.DataDir)
	fmt.Printf("   Backup Directory: %s\n", cfg.BackupDir)
	fmt.Printf("   Log Level: info\n") // Default log level
	fmt.Printf("   Authentication: %t\n", cfg.EnableAuth)
	fmt.Printf("   Max Content Size: %d bytes\n", cfg.MaxContentSize)
	fmt.Printf("   Backup Interval: %v\n", cfg.BackupInterval)
	fmt.Printf("   GC Interval: %v\n", cfg.GCInterval)
	fmt.Printf("   Adaptive GC: %t\n", cfg.EnableAdaptiveGC)
	fmt.Printf("   Emergency Recovery: %t\n", cfg.EnableEmergencyRecovery)
	fmt.Printf("   Performance Mode: %v\n", cfg.PerformanceMode)
	fmt.Printf("   Cache Size: %d bytes (%.1f MB)\n", cfg.CacheSize, float64(cfg.CacheSize)/(1024*1024))
	fmt.Printf("   Write Queue Size: %d\n", cfg.WriteQueueSize)
	fmt.Printf("   Write Queue Workers: 1 (single writer for sequential ordering)\n")
	fmt.Printf("   Write Queue Batch Size: %d\n", cfg.WriteQueueBatchSize)
	fmt.Printf("   Write Queue Batch Timeout: %v\n", cfg.WriteQueueBatchTimeout)
	fmt.Printf("   Note: Adaptive batching and conflict resolution removed for simplified operation\n")

	fmt.Printf("\n🔐 Security & Authentication:\n")
	if cfg.EnableAuth {
		fmt.Printf("   API Authentication: Enabled\n")
	} else {
		fmt.Printf("   API Authentication: Disabled\n")
	}
	fmt.Printf("   Allowed Origins: %v\n", cfg.AllowedOrigins)
	if len(cfg.AllowedIPs) > 0 {
		fmt.Printf("   IP Allowlist: %v\n", cfg.AllowedIPs)
	} else {
		fmt.Printf("   IP Allowlist: All IPs allowed\n")
	}

	fmt.Printf("\n⚡ Performance & Rate Limiting:\n")
	fmt.Printf("   Max Content Size: %d bytes (%.1f MB)\n", cfg.MaxContentSize, float64(cfg.MaxContentSize)/(1024*1024))
	fmt.Printf("   Request Timeout: %v\n", cfg.RequestTimeout)
	fmt.Printf("   Shutdown Timeout: %v\n", cfg.ShutdownTimeout)
	fmt.Printf("   Throttle Limit: %d concurrent requests\n", cfg.ThrottleLimit)
	fmt.Printf("   Throttle Backlog: %d queued requests\n", cfg.ThrottleBacklogLimit)
	fmt.Printf("   Throttle Timeout: %v\n", cfg.ThrottleBacklogTimeout)

	fmt.Printf("\n🗜️ Compression & Security:\n")
	if cfg.EnableCompression {
		fmt.Printf("   Compression: Enabled (Level %d)\n", cfg.CompressionLevel)
		fmt.Printf("   Compression Types: %v\n", cfg.CompressionTypes)
	} else {
		fmt.Printf("   Compression: Disabled\n")
	}
	if cfg.EnableProfiler {
		fmt.Printf("   Profiler: Enabled\n")
	} else {
		fmt.Printf("   Profiler: Disabled\n")
	}

	fmt.Printf("\n📝 Logging Configuration:\n")
	fmt.Printf("   Request Logging: %v\n", cfg.EnableRequestLogging)
	fmt.Printf("   Management Logging: %v\n", cfg.EnableManagementLogging)
	fmt.Printf("   Security Logging: %v\n", cfg.EnableSecurityLogging)
	fmt.Printf("   Critical Logging: %v\n", cfg.EnableCriticalLogging)
	fmt.Printf("   CRUD Logging Suppressed: %v\n", cfg.SuppressCRUDLogging)
	fmt.Println()
}

// Helper functions to get environment variables with defaults

func env(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func envInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func envInt64(key string, defaultValue int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func envBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func envDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func envStringSlice(key string, defaultValue []string) []string {
	if value, exists := os.LookupEnv(key); exists {
		if value == "" {
			return defaultValue
		}
		// Parse comma-separated values
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

func envStringMap(key string, defaultValue map[string]string) map[string]string {
	if value, exists := os.LookupEnv(key); exists {
		if value == "" {
			return defaultValue
		}
		// Parse key=value,key2=value2 format
		result := make(map[string]string)
		pairs := strings.Split(value, ",")
		for _, pair := range pairs {
			if kv := strings.SplitN(strings.TrimSpace(pair), "=", 2); len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				val := strings.TrimSpace(kv[1])
				if key != "" {
					result[key] = val
				}
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

func envFloat64(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}
