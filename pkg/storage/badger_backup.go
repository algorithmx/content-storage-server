package storage

import (
	"fmt"
	"time"
)

// StartBackups starts the automated backup process
func (s *BadgerStorage) StartBackups() error {
	if s.backupManager == nil {
		return fmt.Errorf("backup manager not initialized")
	}
	return s.backupManager.Start()
}

// StopBackups stops the automated backup process
func (s *BadgerStorage) StopBackups() {
	if s.backupManager != nil {
		s.backupManager.Stop()
	}
}

// CreateBackup creates a manual backup immediately
func (s *BadgerStorage) CreateBackup() error {
	if s.backupManager == nil {
		return fmt.Errorf("backup manager not initialized")
	}
	return s.backupManager.CreateBackup()
}

// RestoreFromBackup restores the database from a backup file
func (s *BadgerStorage) RestoreFromBackup(backupPath string) error {
	if s.backupManager == nil {
		return fmt.Errorf("backup manager not initialized")
	}
	return s.backupManager.RestoreFromBackup(backupPath)
}

// VerifyBackup verifies the integrity of a backup file
func (s *BadgerStorage) VerifyBackup(backupPath string) error {
	if s.backupManager == nil {
		return fmt.Errorf("backup manager not initialized")
	}
	return s.backupManager.VerifyBackup(backupPath)
}

// GetBackupStats returns basic backup-related statistics
func (s *BadgerStorage) GetBackupStats() (map[string]interface{}, error) {
	if s.backupManager == nil {
		return nil, fmt.Errorf("backup manager not initialized")
	}

	backups, err := s.backupManager.ListBackups()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"backup_count":     len(backups),
		"last_backup_time": s.backupManager.GetLastBackupTime(),
		"is_running":       s.backupManager.IsRunning(),
		"backup_files":     backups,
	}, nil
}

// ListBackups returns a list of available backup files
func (s *BadgerStorage) ListBackups() ([]string, error) {
	if s.backupManager == nil {
		return nil, fmt.Errorf("backup manager not initialized")
	}
	return s.backupManager.ListBackups()
}

// SetBackupInterval changes the backup interval
func (s *BadgerStorage) SetBackupInterval(interval time.Duration) error {
	if s.backupManager == nil {
		return fmt.Errorf("backup manager not initialized")
	}
	s.backupManager.SetInterval(interval)
	return nil
}

// IsBackupRunning returns whether the backup manager is currently running
func (s *BadgerStorage) IsBackupRunning() bool {
	if s.backupManager == nil {
		return false
	}
	return s.backupManager.IsRunning()
}

// GetLastBackupTime returns the timestamp of the last successful backup
func (s *BadgerStorage) GetLastBackupTime() time.Time {
	if s.backupManager == nil {
		return time.Time{}
	}
	return s.backupManager.GetLastBackupTime()
}
