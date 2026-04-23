package provider

import "sync"

// lazyValue provides thread-safe lazy initialization of a value.
// It replaces the common (sync.Once + data + error) field triple pattern.
type lazyValue[T any] struct {
	once sync.Once
	val  T
	err  error
}

// init loads the value using fn if not already loaded. Returns the stored error.
func (l *lazyValue[T]) init(fn func() (T, error)) error {
	l.once.Do(func() { l.val, l.err = fn() })
	return l.err
}

// v returns the lazily-loaded value. Must be called after a successful init.
func (l *lazyValue[T]) v() T {
	return l.val
}
