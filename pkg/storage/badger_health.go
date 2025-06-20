package storage

// StartHealthMonitoring starts the health monitoring system
func (s *BadgerStorage) StartHealthMonitoring() {
	if s.healthMonitor != nil {
		s.healthMonitor.Start()
	}
}

// GetHealthStatus returns the current health status
func (s *BadgerStorage) GetHealthStatus() map[string]interface{} {
	if s.healthMonitor == nil {
		return map[string]interface{}{"error": "health monitor not initialized"}
	}
	return s.healthMonitor.GetHealthSummary()
}

// GetOverallHealth returns the overall health status
func (s *BadgerStorage) GetOverallHealth() HealthStatus {
	if s.healthMonitor == nil {
		return HealthStatusUnhealthy
	}
	return s.healthMonitor.GetOverallHealth()
}

// IsHealthy returns true if the storage system is healthy
func (s *BadgerStorage) IsHealthy() bool {
	if s.healthMonitor == nil {
		return false
	}
	return s.healthMonitor.GetOverallHealth() == HealthStatusHealthy
}

func (s *BadgerStorage) GetDatabaseHealthMetrics() HealthStatus {
	return s.healthMonitor.CheckDatabaseHealth()
}

// areAllComponentsHealthy checks if all storage components are healthy
func (s *BadgerStorage) areAllComponentsHealthy() bool {
	if s.healthMonitor == nil {
		return false // No health monitor means we can't determine health
	}

	// Get overall health status from the health monitor
	overallHealth := s.healthMonitor.GetOverallHealth()
	return overallHealth == HealthStatusHealthy
}

// StopHealthMonitoring stops the health monitoring system
func (s *BadgerStorage) StopHealthMonitoring() {
	if s.healthMonitor != nil {
		s.healthMonitor.Stop()
	}
}
