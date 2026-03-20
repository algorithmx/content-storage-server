package storage

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"content-storage-server/pkg/models"
)

func TestCapacityDoublingRaceWithWorker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tempDir, err := os.MkdirTemp("", "badger-race-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	initialSize := 10
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    5,
		BatchTimeout: 50 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(10)
	expires := time.Now().Add(1 * time.Hour)

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				content := &models.Content{
					ID:        fmt.Sprintf("worker-test-%d", time.Now().UnixNano()),
					Type:      "text/plain",
					Data:      "test data",
					ExpiresAt: &expires,
				}
				_ = qwb.QueueWrite(content)
				runtime.Gosched()
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			qwb.DoubleQueueCapacity()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(stopCh)
	wg.Wait()

	t.Logf("Completed without deadlock")
}

func TestCapacityDoublingConcurrentWritesAndDoubles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tempDir, err := os.MkdirTemp("", "badger-concurrent-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	initialSize := 5
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    10,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(20)
	expires := time.Now().Add(1 * time.Hour)

	var wg sync.WaitGroup
	numWriters := 20
	opsPerWriter := 100

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerWriter; j++ {
				content := &models.Content{
					ID:        fmt.Sprintf("concurrent-%d-%d", id, j),
					Type:      "text/plain",
					Data:      "test data",
					ExpiresAt: &expires,
				}
				err := qwb.QueueWrite(content)
				if err != nil {
					t.Logf("Write error from goroutine %d: %v", id, err)
				}
				runtime.Gosched()
			}
		}(i)
	}

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qwb.DoubleQueueCapacity()
		}()
	}

	wg.Wait()

	metrics := qwb.GetMetrics()
	t.Logf("Total queued: %d, Total processed: %d, Failed: %d",
		metrics.TotalQueued, metrics.TotalProcessed, metrics.TotalErrors)
}

func TestCapacityDoublingDataRaceBetweenWorkerAndDoubler(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-worker-race-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    5,
		BatchSize:    10,
		BatchTimeout: 200 * time.Millisecond,
	})
	defer qwb.Stop()

	expires := time.Now().Add(1 * time.Hour)

	var wg sync.WaitGroup
	iterations := 100

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			for j := 0; j < 10; j++ {
				content := &models.Content{
					ID:        fmt.Sprintf("racer-%d-%d", i, j),
					Type:      "text/plain",
					Data:      "data",
					ExpiresAt: &expires,
				}
				_ = qwb.QueueWrite(content)
			}
			runtime.Gosched()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			qwb.DoubleQueueCapacity()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()

	t.Logf("Completed %d iterations with concurrent writes", iterations)
}

func TestCapacityDoublingNoDataRaceMultipleRounds(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	for round := 0; round < 5; round++ {
		tempDir, err := os.MkdirTemp("", fmt.Sprintf("badger-round-%d-*", round))
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}

		opts := badger.DefaultOptions(tempDir)
		opts.Logger = nil
		db, err := badger.Open(opts)
		if err != nil {
			os.RemoveAll(tempDir)
			t.Fatalf("Failed to open BadgerDB: %v", err)
		}

		initialSize := 10
		qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
			QueueSize:    initialSize,
			BatchSize:    5,
			BatchTimeout: 50 * time.Millisecond,
		})

		expires := time.Now().Add(1 * time.Hour)
		var wg sync.WaitGroup

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					content := &models.Content{
						ID:        fmt.Sprintf("round-%d-%d", i, j),
						Type:      "text/plain",
						Data:      "test",
						ExpiresAt: &expires,
					}
					_ = qwb.QueueWrite(content)
				}
			}()
		}

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				qwb.DoubleQueueCapacity()
			}()
		}

		wg.Wait()
		qwb.Stop()
		db.Close()
		os.RemoveAll(tempDir)

		t.Logf("Round %d completed", round)
	}
}

func TestQueueWriteDuringCapacityDoubling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "badger-write-during-double-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	initialSize := 5
	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    initialSize,
		BatchSize:    10,
		BatchTimeout: 100 * time.Millisecond,
	})
	defer qwb.Stop()

	qwb.SetMaxMultiplier(50)
	expires := time.Now().Add(1 * time.Hour)

	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for i := 0; i < 200; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() { <-sem }()

			content := &models.Content{
				ID:        fmt.Sprintf("item-%d", n),
				Type:      "text/plain",
				Data:      "test data",
				ExpiresAt: &expires,
			}

			if err := qwb.QueueWrite(content); err != nil {
				t.Logf("QueueWrite error at %d: %v", n, err)
			}
		}(i)
	}

	wg.Wait()

	metrics := qwb.GetMetrics()
	pending := qwb.GetPendingContentCount()
	t.Logf("Completed - Total queued: %d, Processed: %d, Pending: %d, Errors: %d",
		metrics.TotalQueued, metrics.TotalProcessed, pending, metrics.TotalErrors)

	if pending < 0 {
		t.Errorf("BUG: Negative pending count: %d", pending)
	}
	if metrics.TotalQueued < 0 {
		t.Errorf("BUG: Negative total queued: %d", metrics.TotalQueued)
	}
}

func TestCapacityDoublingStressWithImmediateWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "badger-stress-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := badger.DefaultOptions(tempDir)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	qwb := NewQueuedWriteBatch(db, QueuedWriteBatchOptions{
		QueueSize:    5,
		BatchSize:    20,
		BatchTimeout: 50 * time.Millisecond,
	})
	defer qwb.Stop()

	expires := time.Now().Add(1 * time.Hour)

	var wg sync.WaitGroup
	start := time.Now()

	for round := 0; round < 10; round++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				content := &models.Content{
					ID:        fmt.Sprintf("stress-%d-%d", r, i),
					Type:      "text/plain",
					Data:      "data",
					ExpiresAt: &expires,
				}
				_ = qwb.QueueWrite(content)
			}
		}(round)

		wg.Add(1)
		go func() {
			defer wg.Done()
			qwb.DoubleQueueCapacity()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	metrics := qwb.GetMetrics()
	t.Logf("Stress test completed in %v - Queued: %d, Processed: %d, Errors: %d",
		elapsed, metrics.TotalQueued, metrics.TotalProcessed, metrics.TotalErrors)
}
