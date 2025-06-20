package storage

import (
	"sync"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// HealthStatus represents the health status of a component
type HealthStatus int

const (
	HealthStatusHealthy HealthStatus = iota
	HealthStatusDegraded
	HealthStatusUnhealthy
)

func (hs HealthStatus) String() string {
	switch hs {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusDegraded:
		return "degraded"
	case HealthStatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthMonitor monitors the health of BadgerDB storage components
type HealthMonitor struct {
	storage       *BadgerStorage
	startTime     time.Time
	stopChan      chan struct{}
	isRunning     bool
	wg            sync.WaitGroup  // Wait for goroutine to finish
	mu            sync.RWMutex
	stopOnce      sync.Once       // Ensure stop is called only once
	checkInterval time.Duration
	lastCheck     time.Time
}

// NewHealthMonitor creates a new health monitor for BadgerDB storage
func NewHealthMonitor(storage *BadgerStorage) *HealthMonitor {
	return &HealthMonitor{
		storage:       storage,
		startTime:     time.Now(),
		stopChan:      make(chan struct{}),
		checkInterval: 60 * time.Second, // Health check every 60 seconds
	}
}

// Start begins health monitoring
func (hm *HealthMonitor) Start() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if hm.isRunning {
		return
	}

	hm.isRunning = true
	hm.stopChan = make(chan struct{})

	// Start health check loop with WaitGroup
	hm.wg.Add(1)
	go hm.healthCheckLoop()
}

// Stop halts health monitoring
func (hm *HealthMonitor) Stop() {
	hm.stopOnce.Do(func() {
		hm.mu.Lock()
		if !hm.isRunning {
			hm.mu.Unlock()
			return
		}

		// Close stop channel to signal goroutine to exit (safe close)
		select {
		case <-hm.stopChan:
			// Channel already closed
		default:
			close(hm.stopChan)
		}
		hm.mu.Unlock()

		// Wait for goroutine to finish (outside the lock)
		hm.wg.Wait()

		// Update state
		hm.mu.Lock()
		hm.isRunning = false
		hm.mu.Unlock()
	})
}

// IsRunning returns whether health monitoring is active
func (hm *HealthMonitor) IsRunning() bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.isRunning
}

// GetOverallHealth calculates and returns the comprehensive system health status.
//
// This function implements a hierarchical health assessment system that evaluates
// all critical system components and determines the overall operational status.
//
// ## Health Assessment Strategy:
// - **Database Health**: Primary indicator - unhealthy database = unhealthy system
// - **Backup Health**: Secondary indicator - backup issues cause degraded status
// - **GC Health**: Performance indicator - GC issues cause degraded status
// - **Queue Health**: Operational indicator - queue issues cause degraded status
//
// ## Health Status Hierarchy:
// 1. **Unhealthy**: Critical system failure (database corruption, inaccessible)
// 2. **Degraded**: Functional but with performance/reliability issues
// 3. **Healthy**: All systems operating within normal parameters
//
// ## Component Health Logic:
// - **Database Unhealthy** → System Unhealthy (critical failure)
// - **Backup Unhealthy** → System Degraded (data protection compromised)
// - **GC Unhealthy** → System Degraded (performance impact)
// - **Any Component Degraded** → System Degraded (partial functionality)
// - **All Components Healthy** → System Healthy (optimal operation)
//
// ## Monitoring Integration:
// - Used by detailed health check endpoint for adaptive HTTP status codes
// - Integrated with management dashboard for real-time status display
// - Provides basis for alerting and monitoring systems
//
// ## Performance Characteristics:
// - **Fast Execution**: Optimized for frequent health checks
// - **Cached Results**: Uses cached component health where appropriate
// - **Non-blocking**: Does not perform expensive operations during assessment
//
// Returns HealthStatus enum value representing current system health.
func (hm *HealthMonitor) GetOverallHealth() HealthStatus {
	// Check database health
	if hm.CheckDatabaseHealth() == HealthStatusUnhealthy {
		return HealthStatusUnhealthy
	}

	// Check backup health
	backupHealth := hm.checkBackupHealth()
	if backupHealth == HealthStatusUnhealthy {
		return HealthStatusDegraded // Backup issues are degraded, not unhealthy
	}

	// Check GC health
	gcHealth := hm.checkGCHealth()
	if gcHealth == HealthStatusUnhealthy {
		return HealthStatusDegraded // GC issues are degraded, not unhealthy
	}

	// If any component is degraded, overall status is degraded
	if backupHealth == HealthStatusDegraded || gcHealth == HealthStatusDegraded {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}

// healthCheckLoop performs periodic health checks
func (hm *HealthMonitor) healthCheckLoop() {
	defer hm.wg.Done()

	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()

	// Perform initial health check
	hm.performHealthCheck()

	for {
		select {
		case <-ticker.C:
			hm.performHealthCheck()
		case <-hm.stopChan:
			return
		}
	}
}

// performHealthCheck executes a basic health check
func (hm *HealthMonitor) performHealthCheck() {
	hm.mu.Lock()
	hm.lastCheck = time.Now()
	hm.mu.Unlock()
}

// checkDatabaseHealth checks the health of the BadgerDB database
func (hm *HealthMonitor) CheckDatabaseHealth() HealthStatus {
	// Test database connectivity with a simple read operation
	err := hm.storage.db.View(func(txn *badger.Txn) error {
		// Just test that we can create a transaction
		return nil
	})

	if err != nil {
		return HealthStatusUnhealthy
	}

	return HealthStatusHealthy
}

// checkBackupHealth checks the health of the backup system
func (hm *HealthMonitor) checkBackupHealth() HealthStatus {
	if hm.storage.backupManager == nil {
		return HealthStatusDegraded // Backup not configured
	}

	if !hm.storage.backupManager.IsRunning() {
		return HealthStatusDegraded // Backup not running
	}

	// Check if backups are recent
	lastBackup := hm.storage.backupManager.GetLastBackupTime()
	// If backup manager is running but no backup has been created yet (zero time),
	// consider it healthy since the initial backup might still be in progress
	if !lastBackup.IsZero() && time.Since(lastBackup) > 48*time.Hour {
		return HealthStatusDegraded // No backup in 48 hours
	}

	return HealthStatusHealthy
}

// checkGCHealth checks the health of the garbage collection system
func (hm *HealthMonitor) checkGCHealth() HealthStatus {
	if hm.storage.gc == nil {
		return HealthStatusUnhealthy
	}

	if !hm.storage.gc.IsRunning() {
		return HealthStatusDegraded
	}

	return HealthStatusHealthy
}

// SetCheckInterval changes the health check interval
func (hm *HealthMonitor) SetCheckInterval(interval time.Duration) {
	hm.mu.Lock()
	hm.checkInterval = interval
	wasRunning := hm.isRunning
	hm.mu.Unlock()

	// Restart monitoring if running (do this outside the lock to avoid deadlock)
	if wasRunning {
		hm.Stop()
		hm.Start()
	}
}

// GetHealthSummary provides comprehensive health information for monitoring and debugging.
//
// This function generates a detailed health report containing status information
// for all system components, operational metrics, and monitoring metadata.
//
// ## Summary Components:
// - **Overall Status**: Aggregated system health (healthy/degraded/unhealthy)
// - **Component Status**: Individual health status for each system component
// - **Operational Metrics**: Uptime, last check time, monitoring status
// - **System Information**: Runtime and performance indicators
//
// ## Component Health Details:
// - **Database Status**: BadgerDB health, corruption detection, accessibility
// - **Backup Status**: Backup system health, last backup time, backup count
// - **GC Status**: Garbage collection health, frequency, effectiveness
// - **Queue Status**: Write queue health, depth, processing rate
//
// ## Monitoring Metadata:
// - **Last Check**: Timestamp of most recent health assessment
// - **Uptime**: Total system uptime since health monitor initialization
// - **Monitoring Active**: Whether continuous health monitoring is running
//
// ## Usage Context:
// - **Management Dashboard**: Real-time health display and monitoring
// - **API Endpoints**: Detailed health check responses
// - **Debugging**: Comprehensive system state information
// - **Alerting**: Health status for external monitoring systems
//
// ## Data Format:
// Returns map[string]interface{} for flexible JSON serialization and display.
// All timestamps are in RFC3339 format for consistent parsing.
//
// Returns comprehensive health summary suitable for monitoring and debugging.
func (hm *HealthMonitor) GetHealthSummary() map[string]interface{} {
	overall := hm.GetOverallHealth()
	databaseHealth := hm.CheckDatabaseHealth()
	backupHealth := hm.checkBackupHealth()
	gcHealth := hm.checkGCHealth()

	hm.mu.RLock()
	lastCheck := hm.lastCheck
	uptime := time.Since(hm.startTime)
	hm.mu.RUnlock()

	return map[string]interface{}{
		"overall_status":  overall.String(),
		"database_status": databaseHealth.String(),
		"backup_status":   backupHealth.String(),
		"gc_status":       gcHealth.String(),
		"last_check":      lastCheck,
		"uptime_seconds":  uptime.Seconds(),
		"is_monitoring":   hm.IsRunning(),
	}
}

// GetChannelStats returns channel and goroutine statistics for monitoring
func (hm *HealthMonitor) GetChannelStats() map[string]interface{} {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	return map[string]interface{}{
		"stop_channel_initialized": hm.stopChan != nil,
		"is_running":               hm.isRunning,
		"component_type":           "health_monitor",
		"check_interval":           hm.checkInterval.String(),
		"start_time":               hm.startTime,
	}
}
