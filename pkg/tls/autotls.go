// Package tls provides TLS/HTTPS functionality for the content storage server.
// It handles automatic certificate management using Let's Encrypt through Echo's AutoTLS feature.
package tls

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"content-storage-server/internal/handlers"
	"content-storage-server/pkg/config"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/acme/autocert"
)



// SetupAndStartAutoTLS configures and starts the Echo server with automatic TLS certificate management.
// This function combines AutoTLS setup and server startup for a streamlined experience.
//
// Features:
// - Automatic Let's Encrypt certificate provisioning
// - Certificate caching to avoid rate limits
// - Host policy enforcement for security
// - Optional HTTP to HTTPS redirect
// - Graceful error handling and logging
//
// The function blocks until the server shuts down or encounters an error.
func SetupAndStartAutoTLS(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger) {
	// Configure AutoTLS before starting the server
	SetupAutoTLS(e, cfg, appLogger)

	// Display server startup information
	logAutoTLSStartup(cfg, appLogger)

	// Start AutoTLS server (this blocks until shutdown)
	startAutoTLSServer(e, cfg, appLogger)
}

// SetupAutoTLS configures automatic TLS certificate management using Let's Encrypt.
// This function sets up the AutoTLS manager with appropriate security policies and caching.
// It is exported so it can be used separately from server startup.
func SetupAutoTLS(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger) {
	// Configure AutoTLS manager for automatic certificate management
	e.AutoTLSManager.Prompt = autocert.AcceptTOS                    // Accept Let's Encrypt Terms of Service
	e.AutoTLSManager.Cache = autocert.DirCache(cfg.TLSCacheDir)     // Cache certificates to avoid rate limits

	// Configure host policy for security (only allow specified domains)
	if len(cfg.TLSHosts) > 0 {
		e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(cfg.TLSHosts...)
		appLogger.Info("AutoTLS configured with host whitelist",
			zap.Strings("hosts", cfg.TLSHosts),
			zap.String("cache_dir", cfg.TLSCacheDir))
	} else {
		appLogger.Warn("AutoTLS configured without host restrictions - suitable for development only",
			zap.String("cache_dir", cfg.TLSCacheDir),
			zap.String("security_note", "Use TLS_HOSTS in production"))
	}

	// Add HTTPS redirect middleware if HTTPS-only mode is enabled
	if cfg.EnableHTTPSOnly {
		e.Pre(middleware.HTTPSRedirect())
		appLogger.Info("HTTPS redirect enabled - all HTTP traffic will be redirected to HTTPS")
	}

	appLogger.Info("AutoTLS configuration completed successfully")
}


// StartAutoTLSServerWithShutdown configures and starts AutoTLS server with proper shutdown handling
func StartAutoTLSServerWithShutdown(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger, shutdownHandler *handlers.ShutdownHandler, storage interface{}) {
	// Phase 1

	// Configure AutoTLS before starting
	SetupAutoTLS(e, cfg, appLogger)

	appLogger.Info("Starting HTTPS server with AutoTLS",
		zap.String("port", cfg.TLSPort),
		zap.String("cache_dir", cfg.TLSCacheDir),
		zap.Bool("https_only", cfg.EnableHTTPSOnly))

	// Start AutoTLS server in background goroutine
	go func() {
		e.HideBanner = true // Disable Echo startup banner
		err := e.StartAutoTLS(fmt.Sprintf(":%s", cfg.TLSPort))
		if err != nil && err != http.ErrServerClosed {
			appLogger.Fatal("AutoTLS server failed to start", zap.Error(err))
		}
	}()

	displaySuccessInfo()

	// Phase 2: Setup sophisticated shutdown handling for HTTPS server

	// Setup signal handling
	quit := make(chan os.Signal, 2) // Buffer for detecting double signals
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	var firstSignal os.Signal
	var firstSignalTime time.Time

	for {
		sig := <-quit
		now := time.Now()

		// IMMEDIATELY set shutdown state to refuse new requests
		shutdownHandler.InitiateShutdown()

		if firstSignal == nil {
			fmt.Println("=== CTRL+C received, wait for 3s ...")
		}

		// Determine shutdown type
		isEmergency := false

		switch sig {
		case syscall.SIGUSR1:
			// SIGUSR1 always triggers emergency shutdown
			isEmergency = true
			appLogger.Error("Emergency shutdown initiated", zap.String("signal", "SIGUSR1"))
			fmt.Println("=== SIGUSR1")

		case syscall.SIGINT, syscall.SIGTERM:
			if firstSignal == nil {
				// First signal
				firstSignal = sig
				firstSignalTime = now

				// Wait for potential second signal
				go func() {
					time.Sleep(3 * time.Second)
					// If we're still waiting, proceed with graceful shutdown
					select {
					case quit <- syscall.SIGTERM: // Send a delayed graceful signal
						fmt.Println("=== SIGTERM")
					default:
						fmt.Println("=== SIGTERM (not accepted)")
					}
				}()
				continue

			} else {
				// Second signal - check timing
				timeSinceFirst := now.Sub(firstSignalTime)
				if timeSinceFirst <= 3*time.Second {
					isEmergency = true
					appLogger.Error("Emergency shutdown initiated", zap.String("trigger", "double_signal"))
				}
			}
		}

		// Execute shutdown
		if isEmergency {
			performEmergencyTLSShutdown(e, cfg, appLogger, storage)
		} else {
			performGracefulTLSShutdown(e, cfg, appLogger)
		}
		return
	}
}

// performGracefulTLSShutdown performs a graceful TLS server shutdown
func performGracefulTLSShutdown(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger) {
	startTime := time.Now()

	// Create shutdown deadline to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Create a channel to track shutdown completion
	shutdownDone := make(chan error, 1)

	// Start shutdown in goroutine to monitor progress
	go func() {
		err := e.Shutdown(ctx)
		shutdownDone <- err
	}()

	// Monitor shutdown progress
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-shutdownDone:
			totalDuration := time.Since(startTime)
			if err != nil {
				appLogger.Error("HTTPS server shutdown failed", zap.Error(err), zap.Duration("duration", totalDuration))
				fmt.Printf(" HTTPS server shutdown - FAILED (%v) [%v]\n", err, totalDuration)
			} else {
				appLogger.Info("HTTPS server shutdown completed", zap.Duration("duration", totalDuration))
				fmt.Printf(" HTTPS server shutdown - SUCCESS [%v]\n", totalDuration)
			}
			return

		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := cfg.ShutdownTimeout - elapsed
			fmt.Printf("HTTPS server shutdown - TIMEOUT [%v] | [%v]\n", elapsed, remaining)

		case <-ctx.Done():
			elapsed := time.Since(startTime)
			appLogger.Error("HTTPS server shutdown timeout", zap.Duration("elapsed", elapsed))
			fmt.Printf("HTTPS server shutdown - TIMEOUT (%v) [%v]\n", ctx.Err(), elapsed)
			return
		}
	}
}

// performEmergencyTLSShutdown performs an emergency TLS server shutdown
func performEmergencyTLSShutdown(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger, storage interface{}) {
	startTime := time.Now()

	// Step 1: Perform emergency storage shutdown first (data preservation)
	if storageInstance, ok := storage.(interface{ EmergencyShutdown() error }); ok {
		if err := storageInstance.EmergencyShutdown(); err != nil {
			appLogger.Error("Emergency storage shutdown failed", zap.Error(err))
			fmt.Printf("Emergency storage shutdown - FAILED (%v)\n", err)
		}
	}

	// Step 2: Force immediate server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		elapsed := time.Since(startTime)
		appLogger.Error("HTTPS server emergency shutdown failed", zap.Error(err), zap.Duration("duration", elapsed))
		fmt.Printf("HTTPS server emergency shutdown - FAILED (%v) [%v]\n", err, elapsed)
	} else {
		elapsed := time.Since(startTime)
		appLogger.Info("HTTPS server emergency shutdown completed", zap.Duration("duration", elapsed))
		fmt.Printf("HTTPS server emergency shutdown - SUCCESS [%v]\n", elapsed)
	}

	// Step 3: Exit immediately (abandon server)
	fmt.Println("🚨 EMERGENCY SHUTDOWN: Server abandoned")
	os.Exit(1)
}

// startAutoTLSServer starts the Echo server with AutoTLS enabled.
// This function blocks until the server shuts down or encounters a fatal error.
func startAutoTLSServer(e *echo.Echo, cfg *config.Config, appLogger *zap.Logger) {
	appLogger.Info("Starting HTTPS server with AutoTLS - server will block until shutdown")

	// Disable Echo startup banner
	e.HideBanner = true

	// Start AutoTLS server (this blocks until shutdown)
	if err := e.StartAutoTLS(fmt.Sprintf(":%s", cfg.TLSPort)); err != nil && err != http.ErrServerClosed {
		appLogger.Fatal("AutoTLS server failed to start",
			zap.Error(err),
			zap.String("port", cfg.TLSPort),
			zap.String("troubleshooting", "Check port availability, DNS configuration, and Let's Encrypt connectivity"))
	}

	appLogger.Info("AutoTLS server shutdown completed")
}

// logAutoTLSStartup logs comprehensive startup information for AutoTLS configuration.
// This helps with debugging and operational visibility.
func logAutoTLSStartup(cfg *config.Config, appLogger *zap.Logger) {
	appLogger.Info("AutoTLS server startup configuration",
		zap.String("port", cfg.TLSPort),
		zap.String("cache_dir", cfg.TLSCacheDir),
		zap.Bool("https_only", cfg.EnableHTTPSOnly),
		zap.Int("host_count", len(cfg.TLSHosts)),
		zap.String("certificate_authority", "Let's Encrypt"),
		zap.String("protocol", "ACME HTTP-01 challenge"))

	if len(cfg.TLSHosts) > 0 {
		appLogger.Info("Production mode: Host whitelist active", zap.Strings("allowed_hosts", cfg.TLSHosts))
	} else {
		appLogger.Warn("Development mode: No host restrictions - certificates will be issued for any domain")
	}

	// Log important operational notes
	appLogger.Info("AutoTLS operational notes",
		zap.String("certificate_renewal", "automatic"),
		zap.String("cache_persistence", "required for rate limit compliance"),
		zap.String("port_requirements", "443 for HTTPS, 80 for ACME challenges"),
		zap.String("dns_requirements", "domain must resolve to this server"))
}

// ValidateAutoTLSConfig validates the AutoTLS configuration for common issues.
// This function can be called before starting the server to catch configuration problems early.
func ValidateAutoTLSConfig(cfg *config.Config) error {
	// Validate TLS port
	if cfg.TLSPort == "" {
		return fmt.Errorf("TLS_PORT cannot be empty when TLS is enabled")
	}

	// Validate cache directory
	if cfg.TLSCacheDir == "" {
		return fmt.Errorf("TLS_CACHE_DIR cannot be empty when TLS is enabled")
	}

	// Warn about security implications
	if len(cfg.TLSHosts) == 0 {
		// This is not an error, but worth noting
		return nil
	}

	// Validate host format (basic validation)
	for _, host := range cfg.TLSHosts {
		if host == "" {
			return fmt.Errorf("empty hostname in TLS_HOSTS list")
		}
		// Additional hostname validation could be added here
	}

	return nil
}

// GetAutoTLSStatus returns information about the current AutoTLS configuration.
// This can be useful for health checks and monitoring.
func GetAutoTLSStatus(cfg *config.Config) map[string]interface{} {
	status := map[string]interface{}{
		"enabled":           cfg.EnableTLS,
		"port":              cfg.TLSPort,
		"cache_directory":   cfg.TLSCacheDir,
		"https_only":        cfg.EnableHTTPSOnly,
		"host_count":        len(cfg.TLSHosts),
		"hosts":             cfg.TLSHosts,
		"certificate_authority": "Let's Encrypt",
		"auto_renewal":      true,
	}

	if len(cfg.TLSHosts) == 0 {
		status["security_mode"] = "development"
		status["host_policy"] = "unrestricted"
	} else {
		status["security_mode"] = "production"
		status["host_policy"] = "whitelist"
	}

	return status
}


func displaySuccessInfo() {
	fmt.Println("🟢 Server started successfully.")
	fmt.Println()
}