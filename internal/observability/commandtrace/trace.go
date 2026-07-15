package commandtrace

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type contextKey struct{}

var noopFinish = func() {}

type Trace struct {
	mu         sync.Mutex
	phases     map[string]Stats
	operations map[string]Stats
	errorType  string
}

type Stats struct {
	Name  string
	Count int
	Total time.Duration
	Max   time.Duration
}

type Snapshot struct {
	// Phases are the request's non-overlapping top-level wall-clock segments.
	// Only phases are used to calculate accounted and unattributed duration.
	Phases []Stats
	// Operations are inclusive diagnostics: nested and parallel operations may
	// overlap, so their totals must never be added to derive request wall time.
	Operations []Stats
	// ErrorType is the latest request-boundary failure classification. It is
	// intentionally a Go type or fixed sentinel (for example "panic"), never an
	// error message, so command summaries remain low-cardinality and leak-free.
	ErrorType string
}

func WithTrace(ctx context.Context) (context.Context, *Trace) {
	if ctx == nil {
		ctx = context.Background()
	}
	if trace := FromContext(ctx); trace != nil {
		return ctx, trace
	}
	trace := &Trace{
		phases:     make(map[string]Stats),
		operations: make(map[string]Stats),
	}
	return context.WithValue(ctx, contextKey{}, trace), trace
}

// WithNewTrace installs an independent trace even when ctx already carries
// one. Shared work can use it and merge the completed operation snapshot into
// every waiting request without tying the work to the first request's trace.
func WithNewTrace(ctx context.Context) (context.Context, *Trace) {
	if ctx == nil {
		ctx = context.Background()
	}
	trace := &Trace{
		phases:     make(map[string]Stats),
		operations: make(map[string]Stats),
	}
	return context.WithValue(ctx, contextKey{}, trace), trace
}

func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(contextKey{}).(*Trace)
	return trace
}

func MeasurePhase(ctx context.Context, name string) func() {
	return measure(ctx, name, true)
}

func MeasureOperation(ctx context.Context, name string) func() {
	return measure(ctx, name, false)
}

func measure(ctx context.Context, name string, phase bool) func() {
	if FromContext(ctx) == nil {
		return noopFinish
	}
	startedAt := time.Now()
	var once sync.Once
	return func() {
		once.Do(func() {
			if phase {
				RecordPhase(ctx, name, time.Since(startedAt))
				return
			}
			RecordOperation(ctx, name, time.Since(startedAt))
		})
	}
}

func RecordPhase(ctx context.Context, name string, elapsed time.Duration) {
	if trace := FromContext(ctx); trace != nil {
		trace.record(name, elapsed, true)
	}
}

func RecordOperation(ctx context.Context, name string, elapsed time.Duration) {
	if trace := FromContext(ctx); trace != nil {
		trace.record(name, elapsed, false)
	}
}

// SetErrorType records the latest request-boundary failure classification.
// Later transport failures deliberately replace earlier application errors so
// the final command summary describes the failure observed by the client.
func SetErrorType(ctx context.Context, errorType string) {
	trace := FromContext(ctx)
	errorType = strings.TrimSpace(errorType)
	if trace == nil || errorType == "" {
		return
	}
	trace.mu.Lock()
	trace.errorType = errorType
	trace.mu.Unlock()
}

// MergeOperations merges already-aggregated operation statistics into the
// trace carried by ctx.
func MergeOperations(ctx context.Context, stats []Stats) {
	if trace := FromContext(ctx); trace != nil {
		trace.merge(stats, false)
	}
}

func (t *Trace) record(name string, elapsed time.Duration, phase bool) {
	if t == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	target := t.operations
	if phase {
		target = t.phases
	}
	current := target[name]
	current.Name = name
	current.Count++
	current.Total += elapsed
	if elapsed > current.Max {
		current.Max = elapsed
	}
	target[name] = current
}

func (t *Trace) merge(stats []Stats, phase bool) {
	if t == nil || len(stats) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	target := t.operations
	if phase {
		target = t.phases
	}
	if target == nil {
		target = make(map[string]Stats)
		if phase {
			t.phases = target
		} else {
			t.operations = target
		}
	}
	for _, stat := range stats {
		name := strings.TrimSpace(stat.Name)
		if name == "" || stat.Count <= 0 {
			continue
		}
		current := target[name]
		current.Name = name
		current.Count += stat.Count
		if stat.Total > 0 {
			current.Total += stat.Total
		}
		if stat.Max > current.Max {
			current.Max = stat.Max
		}
		target[name] = current
	}
}

func (t *Trace) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return Snapshot{
		Phases:     sortedStats(t.phases),
		Operations: sortedStats(t.operations),
		ErrorType:  t.errorType,
	}
}

func sortedStats(source map[string]Stats) []Stats {
	result := make([]Stats, 0, len(source))
	for _, stat := range source {
		result = append(result, stat)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (s Snapshot) AccountedPhaseDuration() time.Duration {
	var total time.Duration
	for _, phase := range s.Phases {
		total += phase.Total
	}
	return total
}

func (s Snapshot) UnattributedDuration(serverDuration time.Duration) time.Duration {
	unattributed := serverDuration - s.AccountedPhaseDuration()
	if unattributed < 0 {
		return 0
	}
	return unattributed
}

func (s Snapshot) PhaseOverrunDuration(serverDuration time.Duration) time.Duration {
	overrun := s.AccountedPhaseDuration() - serverDuration
	if overrun < 0 {
		return 0
	}
	return overrun
}

func (s Snapshot) PhaseValue() slog.Value {
	return statsLogValue(s.Phases)
}

func (s Snapshot) OperationValue() slog.Value {
	return statsLogValue(s.Operations)
}

func statsLogValue(stats []Stats) slog.Value {
	attrs := make([]slog.Attr, 0, len(stats))
	for _, stat := range stats {
		attrs = append(attrs, slog.Group(stat.Name,
			slog.Int("count", stat.Count),
			slog.Float64("total_ms", Milliseconds(stat.Total)),
			slog.Float64("max_ms", Milliseconds(stat.Max)),
		))
	}
	return slog.GroupValue(attrs...)
}

func Milliseconds(duration time.Duration) float64 {
	return math.Round(float64(duration)/float64(time.Millisecond)*1000) / 1000
}
