package testutil

import (
	"sync"
	"time"
)

// FakeClock is a manually advanced clock for tests of time-based behaviour.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a clock stopped at a fixed instant.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Unix(1_700_000_000, 0)}
}

// Now returns the clock's current instant.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
