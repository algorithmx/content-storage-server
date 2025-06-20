package storage

// GetResourceStats returns statistics about resource usage and component states
func (s *BadgerStorage) GetResourceStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Component states
	stats["component_states"] = s.VerifyCleanShutdown()

	// Database size (actual data)
	if dbSize, err := s.GetDatabaseSize(); err == nil {
		stats["database_data_size_bytes"] = dbSize
	} else {
		stats["database_data_size_error"] = err.Error()
	}

	// Database file size (allocated space)
	if fileSize, err := s.GetDatabaseFileSize(); err == nil {
		stats["database_file_size_bytes"] = fileSize
	} else {
		stats["database_file_size_error"] = err.Error()
	}

	// GC stats
	if s.gc != nil {
		stats["gc_stats"] = s.GetGCStats()
	}

	// Backup stats
	if s.backupManager != nil {
		backupStats, err := s.GetBackupStats()
		if err == nil {
			stats["backup_stats"] = backupStats
		} else {
			stats["backup_stats_error"] = err.Error()
		}
	}



	// Health stats
	if s.healthMonitor != nil {
		stats["health_stats"] = s.GetHealthStatus()
	}

	// Access manager stats (memory leak monitoring)
	if s.accessManager != nil {
		stats["access_manager_stats"] = s.accessManager.GetStats()
	}

	return stats
}

// GetChannelStats returns comprehensive channel statistics for all components
// This is primarily useful for debugging and operational monitoring
func (s *BadgerStorage) GetChannelStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Garbage collector channel stats
	if s.gc != nil {
		stats["garbage_collector"] = s.gc.GetChannelStats()
	}

	// Backup manager channel stats
	if s.backupManager != nil {
		stats["backup_manager"] = s.backupManager.GetChannelStats()
	}

	// Health monitor channel stats
	if s.healthMonitor != nil {
		stats["health_monitor"] = s.healthMonitor.GetChannelStats()
	}

	// Overall system stats
	stats["system"] = map[string]interface{}{
		"total_components":  len(stats) - 1, // Subtract 1 for the system entry itself
		"all_components_ok": s.IsFullyShutdown() || s.areAllComponentsHealthy(),
		"shutdown_verified": s.IsFullyShutdown(),
	}

	return stats
}
