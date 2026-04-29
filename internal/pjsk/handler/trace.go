package handler

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type commandTraceContextKey struct{}

type CommandTrace struct {
	mu     sync.Mutex
	stages map[string]commandTraceStage
}

type commandTraceStage struct {
	count int
	total time.Duration
	max   time.Duration
}

func WithCommandTrace(ctx context.Context) (context.Context, *CommandTrace) {
	if ctx == nil {
		ctx = context.Background()
	}
	if trace := CommandTraceFromContext(ctx); trace != nil {
		return ctx, trace
	}
	trace := &CommandTrace{stages: make(map[string]commandTraceStage)}
	return context.WithValue(ctx, commandTraceContextKey{}, trace), trace
}

func CommandTraceFromContext(ctx context.Context) *CommandTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(commandTraceContextKey{}).(*CommandTrace)
	return trace
}

func recordCommandStage(ctx context.Context, stage string, elapsed time.Duration) {
	trace := CommandTraceFromContext(ctx)
	if trace == nil || stage == "" {
		return
	}
	trace.record(stage, elapsed)
}

func (t *CommandTrace) record(stage string, elapsed time.Duration) {
	if t == nil || stage == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stages == nil {
		t.stages = make(map[string]commandTraceStage)
	}
	current := t.stages[stage]
	current.count++
	current.total += elapsed
	if elapsed > current.max {
		current.max = elapsed
	}
	t.stages[stage] = current
}

func (t *CommandTrace) Summary() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.stages) == 0 {
		return ""
	}
	keys := make([]string, 0, len(t.stages))
	for key := range t.stages {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		stage := t.stages[key]
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(formatStageDuration(stage.total))
		if stage.count > 1 {
			b.WriteString("/")
			b.WriteString(formatStageDuration(stage.max))
			b.WriteString("x")
			b.WriteString(formatStageCount(stage.count))
		}
	}
	return b.String()
}

func formatStageDuration(d time.Duration) string {
	return formatStageCount(int(d.Milliseconds())) + "ms"
}

func formatStageCount(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
