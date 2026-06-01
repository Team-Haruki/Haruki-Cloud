package provider

import "sync"

// lazyValue provides thread-safe lazy initialization of a value.
// It replaces the common (sync.Once + data + error) field triple pattern.
type lazyValue[T any] struct {
	mu     sync.RWMutex
	loaded bool
	val    T
	err    error
}

// init loads the value using fn if not already loaded. Returns the stored error.
func (l *lazyValue[T]) init(fn func() (T, error)) error {
	l.mu.RLock()
	if l.loaded {
		err := l.err
		l.mu.RUnlock()
		return err
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.loaded {
		l.val, l.err = fn()
		l.loaded = true
	}
	return l.err
}

// v returns the lazily-loaded value. Must be called after a successful init.
func (l *lazyValue[T]) v() T {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.val
}

func (l *lazyValue[T]) reset() {
	l.mu.Lock()
	l.loaded = false
	l.err = nil
	l.mu.Unlock()
}
