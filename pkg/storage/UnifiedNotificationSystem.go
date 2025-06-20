package storage

import (
	"sync"
)

// UnifiedNotificationSystem manages all event-driven notifications with a single mutex
// True consolidation: replaces 4 separate fields with 2 fields total
type UnifiedNotificationSystem struct {
	mutex               sync.Mutex                            // Single mutex for all notification operations
	completionWaiters   map[string][]chan struct{}            // content ID -> list of waiting channels
	flushWaiters        []chan struct{}                       // channels waiting for flush completion
}

// NewUnifiedNotificationSystem creates a new unified notification system
func NewUnifiedNotificationSystem() *UnifiedNotificationSystem {
	return &UnifiedNotificationSystem{
		completionWaiters: make(map[string][]chan struct{}),
		flushWaiters:      make([]chan struct{}, 0),
	}
}

// registerCompletionWaiter registers a channel to be notified when a specific content ID completes
func (uns *UnifiedNotificationSystem) registerCompletionWaiter(contentID string, notifyChan chan struct{}) {
	uns.mutex.Lock()
	defer uns.mutex.Unlock()

	uns.completionWaiters[contentID] = append(uns.completionWaiters[contentID], notifyChan)
}

// unregisterCompletionWaiter removes a notification channel for a specific content ID
// Uses swap-with-last optimization for O(1) removal instead of O(n) slice operations
func (uns *UnifiedNotificationSystem) unregisterCompletionWaiter(contentID string, notifyChan chan struct{}) {
	uns.mutex.Lock()
	defer uns.mutex.Unlock()

	waiters := uns.completionWaiters[contentID]
	for i, ch := range waiters {
		if ch == notifyChan {
			// Swap-with-last optimization: O(1) removal without memory allocation
			lastIdx := len(waiters) - 1
			if i != lastIdx {
				waiters[i] = waiters[lastIdx] // Move last element to position i
			}
			// Truncate slice (reuses existing capacity)
			uns.completionWaiters[contentID] = waiters[:lastIdx]

			// Clean up empty slice
			if lastIdx == 0 {
				delete(uns.completionWaiters, contentID)
			}
			break
		}
	}
}

// notifyCompletion notifies all waiters for a specific content ID completion
func (uns *UnifiedNotificationSystem) notifyCompletion(contentID string) {
	uns.mutex.Lock()
	defer uns.mutex.Unlock()

	if waiters, exists := uns.completionWaiters[contentID]; exists {
		// Notify all waiters for this content ID
		for _, ch := range waiters {
			select {
			case ch <- struct{}{}:
			default:
				// Channel might be full or closed, skip
			}
		}
		// Clean up the waiters for this content ID
		delete(uns.completionWaiters, contentID)
	}
}

// registerFlushWaiter registers a channel to be notified when flush completes
func (uns *UnifiedNotificationSystem) registerFlushWaiter(notifyChan chan struct{}) {
	uns.mutex.Lock()
	defer uns.mutex.Unlock()

	uns.flushWaiters = append(uns.flushWaiters, notifyChan)
}

// unregisterFlushWaiter removes a flush notification channel
// Uses swap-with-last optimization for O(1) removal instead of O(n) slice operations
func (uns *UnifiedNotificationSystem) unregisterFlushWaiter(notifyChan chan struct{}) {
	uns.mutex.Lock()
	defer uns.mutex.Unlock()

	for i, ch := range uns.flushWaiters {
		if ch == notifyChan {
			// Swap-with-last optimization: O(1) removal without memory allocation
			lastIdx := len(uns.flushWaiters) - 1
			if i != lastIdx {
				uns.flushWaiters[i] = uns.flushWaiters[lastIdx] // Move last element to position i
			}
			// Truncate slice (reuses existing capacity)
			uns.flushWaiters = uns.flushWaiters[:lastIdx]
			break
		}
	}
}

// checkAndNotifyFlushCompletion checks if all pending items are complete and notifies flush waiters
func (uns *UnifiedNotificationSystem) checkAndNotifyFlushCompletion(pendingItems *CountedSyncMap) {
	uns.mutex.Lock()
	defer uns.mutex.Unlock()

	// Fast path: check if there are any flush waiters
	if len(uns.flushWaiters) == 0 {
		return // No one waiting for flush completion
	}

	// Only notify if all pending items are complete
	if pendingItems.IsEmpty() {
		// Notify all flush waiters
		for _, ch := range uns.flushWaiters {
			select {
			case ch <- struct{}{}:
			default:
				// Channel might be full or closed, skip
			}
		}
		// Clear the flush waiters
		uns.flushWaiters = uns.flushWaiters[:0]
	}
}
