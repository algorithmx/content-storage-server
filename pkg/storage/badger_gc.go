package storage

import (
	"time"
)

// RunGC runs the BadgerDB garbage collection process to reclaim disk space
func (s *BadgerStorage) RunGC() error {
	return s.gc.RunGC()
}

// StartGCLoop starts the garbage collection loop
func (s *BadgerStorage) StartGCLoop(interval time.Duration) {
	s.gc.SetInterval(interval)
	s.gc.Start()
}

// StopGCLoop stops the garbage collection loop
func (s *BadgerStorage) StopGCLoop() {
	s.gc.Stop()
}

// GetGCStats returns garbage collection statistics
func (s *BadgerStorage) GetGCStats() map[string]interface{} {
	if s.gc == nil {
		return map[string]interface{}{"error": "GC not initialized"}
	}

	metrics := s.gc.GetMetrics()
	return map[string]interface{}{
		"total_runs":            metrics.TotalRuns,
		"successful_runs":       metrics.SuccessfulRuns,
		"failed_runs":           metrics.FailedRuns,
		"last_run_time":         metrics.LastRunTime,
		"last_run_duration":     metrics.LastRunDuration.String(),
		"space_reclaimed_bytes": metrics.SpaceReclaimed,
		"expired_items_removed": metrics.ExpiredItemsRemoved,
		"is_running":            s.gc.IsRunning(),
	}
}
