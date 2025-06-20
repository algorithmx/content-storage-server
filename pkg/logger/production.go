package logger

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config provides optimized logging configuration
type Config struct {
	// Performance settings
	DisableCaller     bool // Disable caller information for performance
	DisableStacktrace bool // Disable stacktraces for performance
	SamplingEnabled   bool // Enable sampling to reduce log volume

	// Sampling configuration
	SamplingInitial    int // Initial sampling rate
	SamplingThereafter int // Subsequent sampling rate

	// Async settings
	EnableAsync   bool          // Enable async logging
	BufferSize    int           // Async buffer size
	FlushInterval time.Duration // Async flush interval

	// Output settings
	OutputPaths      []string // Output file paths
	ErrorOutputPaths []string // Error output file paths

	// Level settings
	Level zapcore.Level // Minimum log level
}

// NewLogger creates a highly optimized logger
func NewLogger(config Config) (*zap.Logger, error) {
	// Set defaults
	if config.SamplingInitial == 0 {
		config.SamplingInitial = 100
	}
	if config.SamplingThereafter == 0 {
		config.SamplingThereafter = 100
	}
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 100 * time.Millisecond
	}
	if len(config.OutputPaths) == 0 {
		config.OutputPaths = []string{"stdout"}
	}
	if len(config.ErrorOutputPaths) == 0 {
		config.ErrorOutputPaths = []string{"stderr"}
	}

	// Create optimized zap config
	zapConfig := zap.NewProductionConfig()

	// Performance optimizations
	zapConfig.EncoderConfig.TimeKey = "ts"
	zapConfig.EncoderConfig.LevelKey = "level"
	zapConfig.EncoderConfig.MessageKey = "msg"
	zapConfig.EncoderConfig.EncodeTime = zapcore.EpochTimeEncoder // Faster than RFC3339
	zapConfig.EncoderConfig.EncodeDuration = zapcore.NanosDurationEncoder

	// Disable expensive features if requested
	if config.DisableCaller {
		zapConfig.EncoderConfig.CallerKey = ""
	}
	if config.DisableStacktrace {
		zapConfig.EncoderConfig.StacktraceKey = ""
	}

	// Configure sampling
	if config.SamplingEnabled {
		zapConfig.Sampling = &zap.SamplingConfig{
			Initial:    config.SamplingInitial,
			Thereafter: config.SamplingThereafter,
		}
	} else {
		zapConfig.Sampling = nil
	}

	// Set level
	zapConfig.Level = zap.NewAtomicLevelAt(config.Level)

	// Set output paths
	zapConfig.OutputPaths = config.OutputPaths
	zapConfig.ErrorOutputPaths = config.ErrorOutputPaths

	// Build logger
	zapLog, err := zapConfig.Build(
		zap.AddStacktrace(zapcore.DPanicLevel), // Only stacktrace for critical errors
	)
	if err != nil {
		return nil, err
	}

	return zapLog, nil
}

// GetHighPerformanceConfig returns a configuration optimized for maximum performance
func GetHighPerformanceConfig() Config {
	return Config{
		DisableCaller:      true,                   // Disable for maximum performance
		DisableStacktrace:  true,                   // Disable for maximum performance
		SamplingEnabled:    true,                   // Enable sampling to reduce volume
		SamplingInitial:    1000,                   // Sample 1 in 1000 initially
		SamplingThereafter: 1000,                   // Then 1 in 1000 thereafter
		EnableAsync:        true,                   // Enable async logging
		BufferSize:         4000,                   // Large buffer
		FlushInterval:      200 * time.Millisecond, // Less frequent flushes
		Level:              zapcore.WarnLevel,      // Only warnings and errors
		OutputPaths:        []string{"logs/info.log"},
		ErrorOutputPaths:   []string{"logs/error.log"},
	}
}

// GetBalancedConfig returns a configuration balancing performance and observability
func GetBalancedConfig() Config {
	return Config{
		DisableCaller:      false,                  // Keep caller info
		DisableStacktrace:  true,                   // Disable stacktraces for performance
		SamplingEnabled:    true,                   // Enable moderate sampling
		SamplingInitial:    100,                    // Sample 1 in 100 initially
		SamplingThereafter: 100,                    // Then 1 in 100 thereafter
		EnableAsync:        true,                   // Enable async logging
		BufferSize:         1000,                   // Moderate buffer
		FlushInterval:      100 * time.Millisecond, // Moderate flush interval
		Level:              zapcore.InfoLevel,      // Info level and above
		OutputPaths:        []string{"logs/info.log"},
		ErrorOutputPaths:   []string{"logs/error.log"},
	}
}

// GetDebugConfig returns a configuration for development/debugging
func GetDebugConfig() Config {
	return Config{
		DisableCaller:     false,              // Keep caller info for debugging
		DisableStacktrace: false,              // Keep stacktraces for debugging
		SamplingEnabled:   false,              // No sampling in debug mode
		EnableAsync:       false,              // Synchronous for immediate output
		Level:             zapcore.DebugLevel, // All levels
		OutputPaths:        []string{"logs/info.log"},
		ErrorOutputPaths:   []string{"logs/error.log"},
	}
}

// CreateLoggerFromEnv creates a logger based on environment
func CreateLoggerFromEnv() (*zap.Logger, error) {
	// In a real implementation, you would read environment variables
	// to determine which configuration to use

	// For startup logging, we want to see comprehensive logs in both console and files
	config := GetBalancedConfig()

	return NewLogger(config)
}

// NewDefaultLogger creates a logger with balanced configuration
func NewDefaultLogger() (*zap.Logger, error) {
	config := GetBalancedConfig()
	return NewLogger(config)
}
