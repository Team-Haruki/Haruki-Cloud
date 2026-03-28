//go:build cgo && pjsk_deck_cgo

// Package deck_cgo provides a Go interface to the sekai-deck-recommend-cpp
// C++ engine via CGo.
//
// pool.go — goroutine-safe engine pool for deck_cgo.

package deck_cgo

import (
	"fmt"
	"sync"
)

// Pool holds a fixed number of Engine instances and provides goroutine-safe
// access through Acquire / Release.
type Pool struct {
	engines chan *Engine
	once    sync.Once
	size    int
}

// NewPool creates a pool of n Engine instances, all pre-loaded with the given
// masterdata and music metadata.
//
//   - masterdataDir  — local directory of masterdata JSON files (passed to
//     UpdateMasterdata), OR empty string to skip.
//   - masterdataMap  — in-memory masterdata map (used when masterdataDir == "").
//   - musicmetasPath — local path to music_metas.json, OR empty string.
//   - musicmetasData — in-memory music_metas.json bytes (used when path == "").
//   - region         — e.g. "jp"
//   - n              — pool size (typically == runtime.NumCPU())
func NewPool(
	masterdataDir string,
	masterdataMap map[string][]byte,
	musicmetasPath string,
	musicmetasData []byte,
	region string,
	n int,
) (*Pool, error) {
	if n <= 0 {
		n = 1
	}
	p := &Pool{
		engines: make(chan *Engine, n),
		size:    n,
	}

	for i := 0; i < n; i++ {
		eng, err := NewEngine()
		if err != nil {
			p.closeAll()
			return nil, fmt.Errorf("deck_cgo pool[%d]: %w", i, err)
		}

		// Load masterdata
		if masterdataDir != "" {
			if err := eng.UpdateMasterdata(masterdataDir, region); err != nil {
				eng.Close()
				p.closeAll()
				return nil, fmt.Errorf("deck_cgo pool[%d] masterdata dir: %w", i, err)
			}
		} else if len(masterdataMap) > 0 {
			if err := eng.UpdateMasterdataFromStrings(masterdataMap, region); err != nil {
				eng.Close()
				p.closeAll()
				return nil, fmt.Errorf("deck_cgo pool[%d] masterdata strings: %w", i, err)
			}
		}

		// Load music metas
		if musicmetasPath != "" {
			if err := eng.UpdateMusicmetas(musicmetasPath, region); err != nil {
				eng.Close()
				p.closeAll()
				return nil, fmt.Errorf("deck_cgo pool[%d] musicmetas path: %w", i, err)
			}
		} else if len(musicmetasData) > 0 {
			if err := eng.UpdateMusicmetasFromBytes(musicmetasData, region); err != nil {
				eng.Close()
				p.closeAll()
				return nil, fmt.Errorf("deck_cgo pool[%d] musicmetas bytes: %w", i, err)
			}
		}

		p.engines <- eng
	}

	return p, nil
}

// Acquire checks out an Engine from the pool. The caller MUST call
// Release when done. This call blocks if all engines are busy.
func (p *Pool) Acquire() *Engine {
	return <-p.engines
}

// Release returns an Engine to the pool.
func (p *Pool) Release(eng *Engine) {
	p.engines <- eng
}

// Do is a convenience wrapper: acquires an engine, calls fn, and always
// releases back to the pool.
func (p *Pool) Do(fn func(*Engine) error) error {
	eng := p.Acquire()
	defer p.Release(eng)
	return fn(eng)
}

// Close destroys all engines in the pool. Safe to call once.
func (p *Pool) Close() {
	p.once.Do(p.closeAll)
}

func (p *Pool) closeAll() {
	for {
		select {
		case eng := <-p.engines:
			eng.Close()
		default:
			return
		}
	}
}

// Size returns the total capacity of the pool.
func (p *Pool) Size() int { return p.size }
