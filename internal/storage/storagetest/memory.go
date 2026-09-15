// Package storagetest holds the in-memory Store fake and the backend
// conformance suite shared by every storage backend's tests.
package storagetest

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/storage"
)

// Op is one recorded Memory call.
type Op struct {
	Method string
	Key    storage.Key
}

type memoryObject struct {
	data    []byte
	modTime time.Time
}

// Memory is an in-memory storage.Store with failure injection and a call
// recorder. The Fail* hooks and MaxObjectBytes must be set before the store is
// shared between goroutines. A hook receives the cleaned key; a non-nil error
// is returned instead of performing the operation.
type Memory struct {
	FailGet    func(storage.Key) error
	FailPut    func(storage.Key) error
	FailStat   func(storage.Key) error
	FailDelete func(storage.Key) error
	FailList   func(storage.Key) error
	// MaxObjectBytes bounds Put; NewMemory sets storage.DefaultMaxObjectBytes.
	MaxObjectBytes int64

	mu      sync.Mutex
	objects map[storage.Key]memoryObject
	calls   []Op
}

var _ storage.Store = (*Memory)(nil)

// NewMemory returns an empty Memory store.
func NewMemory() *Memory {
	return &Memory{MaxObjectBytes: storage.DefaultMaxObjectBytes, objects: map[storage.Key]memoryObject{}}
}

// Seed stores each entry directly, bypassing hooks and the recorder. It panics
// on an invalid key because it is only used to set up tests.
func (m *Memory) Seed(entries map[string][]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for raw, data := range entries {
		key, err := storage.CleanKey(raw)
		if err != nil {
			panic(err)
		}
		m.objects[key] = memoryObject{data: slices.Clone(data), modTime: time.Now()}
	}
}

// Calls returns a copy of the recorded calls in order.
func (m *Memory) Calls() []Op {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.calls)
}

func (m *Memory) begin(ctx context.Context, method string, raw storage.Key, hook func(storage.Key) error) (storage.Key, error) {
	m.mu.Lock()
	m.calls = append(m.calls, Op{Method: method, Key: raw})
	m.mu.Unlock()
	key, err := storage.CleanKey(string(raw))
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if hook != nil {
		if err := hook(key); err != nil {
			return "", err
		}
	}
	return key, nil
}

// Get implements storage.Store.
func (m *Memory) Get(ctx context.Context, raw storage.Key) ([]byte, error) {
	key, err := m.begin(ctx, "Get", raw, m.FailGet)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[key]
	if !ok {
		return nil, storage.NotExistError("get", key)
	}
	return slices.Clone(object.data), nil
}

// Put implements storage.Store.
func (m *Memory) Put(ctx context.Context, raw storage.Key, data []byte, _ storage.PutOptions) error {
	key, err := m.begin(ctx, "Put", raw, m.FailPut)
	if err != nil {
		return err
	}
	if limit := m.limit(); int64(len(data)) > limit {
		return storage.TooLargeError("put", key, int64(len(data)), limit)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memoryObject{data: slices.Clone(data), modTime: time.Now()}
	return nil
}

// Stat implements storage.Store.
func (m *Memory) Stat(ctx context.Context, raw storage.Key) (storage.Object, error) {
	key, err := m.begin(ctx, "Stat", raw, m.FailStat)
	if err != nil {
		return storage.Object{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[key]
	if !ok {
		return storage.Object{}, storage.NotExistError("stat", key)
	}
	return describe(key, object), nil
}

// Delete implements storage.Store.
func (m *Memory) Delete(ctx context.Context, raw storage.Key) error {
	key, err := m.begin(ctx, "Delete", raw, m.FailDelete)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

// List implements storage.Store. Objects are visited in key order.
func (m *Memory) List(ctx context.Context, rawPrefix storage.Key, fn func(storage.Object) error) error {
	m.mu.Lock()
	m.calls = append(m.calls, Op{Method: "List", Key: rawPrefix})
	m.mu.Unlock()
	prefix, err := storage.CleanPrefix(string(rawPrefix))
	if err != nil {
		return err
	}
	if m.FailList != nil {
		if err := m.FailList(prefix); err != nil {
			return err
		}
	}
	for _, object := range m.snapshot(prefix) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(object); err != nil {
			return err
		}
	}
	return nil
}

func (m *Memory) snapshot(prefix storage.Key) []storage.Object {
	m.mu.Lock()
	defer m.mu.Unlock()
	objects := make([]storage.Object, 0, len(m.objects))
	for key, object := range m.objects {
		if strings.HasPrefix(string(key), string(prefix)) {
			objects = append(objects, describe(key, object))
		}
	}
	slices.SortFunc(objects, func(a, b storage.Object) int { return strings.Compare(string(a.Key), string(b.Key)) })
	return objects
}

func (m *Memory) limit() int64 {
	if m.MaxObjectBytes <= 0 {
		return storage.DefaultMaxObjectBytes
	}
	return m.MaxObjectBytes
}

func describe(key storage.Key, object memoryObject) storage.Object {
	return storage.Object{Key: key, Size: int64(len(object.data)), ModTime: object.modTime}
}
