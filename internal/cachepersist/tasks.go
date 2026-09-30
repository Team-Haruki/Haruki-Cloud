package cachepersist

import (
	"context"
	"errors"
	"sync"
)

// Tasks ties shared refreshes to an application lifecycle and prevents a new
// refresh from racing past shutdown's drain. Its zero value is ready for use.
type Tasks struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
	pending int
	drained chan struct{}
}

func (t *Tasks) Configure(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.pending != 0 {
		return errors.New("cache lifecycle is already running or closed")
	}
	if t.cancel != nil {
		t.cancel()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	t.ctx, t.cancel = context.WithCancel(ctx)
	return nil
}

func (t *Tasks) Start() (context.Context, func(), error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, nil, errors.New("cache lifecycle is closed")
	}
	if t.ctx == nil {
		t.ctx, t.cancel = context.WithCancel(context.Background())
	}
	if err := t.ctx.Err(); err != nil {
		return nil, nil, err
	}
	if t.pending == 0 {
		t.drained = make(chan struct{})
	}
	t.pending++
	var once sync.Once
	return t.ctx, func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.pending--
			if t.pending == 0 {
				close(t.drained)
			}
		})
	}, nil
}

func (t *Tasks) Close(ctx context.Context) error {
	t.mu.Lock()
	t.closed = true
	if t.cancel != nil {
		t.cancel()
	}
	pending, drained := t.pending, t.drained
	t.mu.Unlock()
	if pending == 0 {
		return nil
	}
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
