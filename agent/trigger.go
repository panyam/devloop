package agent

import (
	"sync"
	"time"
)

// TriggerTracker tracks rule execution frequency for rate limiting
type TriggerTracker struct {
	triggers     []time.Time
	mutex        sync.RWMutex
	backoffLevel int       // Current backoff level
	backoffUntil time.Time // Time until which rule is backing off
	backoffMutex sync.RWMutex
}

// NewTriggerTracker creates a new trigger tracker
func NewTriggerTracker() *TriggerTracker {
	return &TriggerTracker{
		triggers: make([]time.Time, 0),
	}
}

// RecordTrigger records a new trigger event
func (t *TriggerTracker) RecordTrigger() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.triggers = append(t.triggers, time.Now())
}

// GetTriggerCount returns the number of triggers within the specified duration
func (t *TriggerTracker) GetTriggerCount(duration time.Duration) int {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	cutoff := time.Now().Add(-duration)
	count := 0

	// Count triggers within the time window
	for _, trigger := range t.triggers {
		if trigger.After(cutoff) {
			count++
		}
	}

	return count
}

// CleanupOldTriggers removes trigger records older than the specified duration
func (t *TriggerTracker) CleanupOldTriggers(duration time.Duration) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	cutoff := time.Now().Add(-duration)
	validTriggers := make([]time.Time, 0)

	// Keep only triggers within the time window
	for _, trigger := range t.triggers {
		if trigger.After(cutoff) {
			validTriggers = append(validTriggers, trigger)
		}
	}

	t.triggers = validTriggers
}

// GetLastTrigger returns the timestamp of the most recent trigger
func (t *TriggerTracker) GetLastTrigger() *time.Time {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if len(t.triggers) == 0 {
		return nil
	}

	return &t.triggers[len(t.triggers)-1]
}

// GetTriggerRate returns the current trigger rate (triggers per minute)
func (t *TriggerTracker) GetTriggerRate() float64 {
	count := t.GetTriggerCount(time.Minute)
	return float64(count)
}

// IsInBackoff checks if the rule is currently in backoff period
func (t *TriggerTracker) IsInBackoff() bool {
	t.backoffMutex.RLock()
	defer t.backoffMutex.RUnlock()
	return time.Now().Before(t.backoffUntil)
}

// GetBackoffLevel returns the current backoff level
func (t *TriggerTracker) GetBackoffLevel() int {
	t.backoffMutex.RLock()
	defer t.backoffMutex.RUnlock()
	return t.backoffLevel
}

// SetBackoff sets the backoff period based on the current level
func (t *TriggerTracker) SetBackoff() {
	t.backoffMutex.Lock()
	defer t.backoffMutex.Unlock()

	t.backoffLevel++

	// Exponential backoff: 2^level seconds, capped at 60 seconds
	backoffDuration := time.Duration(1<<uint(t.backoffLevel)) * time.Second
	if backoffDuration > 60*time.Second {
		backoffDuration = 60 * time.Second
	}

	t.backoffUntil = time.Now().Add(backoffDuration)
}

// ResetBackoff resets the backoff level if enough time has passed
func (t *TriggerTracker) ResetBackoff() {
	t.backoffMutex.Lock()
	defer t.backoffMutex.Unlock()

	// Reset backoff if we haven't triggered in the last 2 minutes
	if time.Since(t.backoffUntil) > 2*time.Minute {
		t.backoffLevel = 0
		t.backoffUntil = time.Time{}
	}
}
