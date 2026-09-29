package cachefill

import (
	"context"
	"errors"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
)

func TestCacheFillTimersPreserveCancellationAndBackoff(t *testing.T) {
	var group Group
	base, cancel := context.WithCancel(t.Context())
	ctx, trace := commandtrace.WithTrace(base)
	called := false
	err := group.Do(ctx, "private_table_key", func(context.Context) error { called = true; cancel(); return nil })
	if err != nil || !called {
		t.Fatalf("synchronous fill semantics changed: called=%t err=%v", called, err)
	}
	operations := map[string]int{}
	for _, operation := range trace.Snapshot().Operations {
		operations[operation.Name] = operation.Count
	}
	if operations["masterdata.cachefill_wait"] != 1 || operations["masterdata.cachefill_canceled"] != 1 {
		t.Fatalf("operations=%v", operations)
	}
	raw, err := json.Marshal(trace.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private_table_key") {
		t.Fatalf("trace leaked key: %s", raw)
	}
	ctx, trace = commandtrace.WithTrace(t.Context())
	boom := errors.New("private_failure_value")
	if err := group.Do(ctx, "private_table_key", func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if err := group.Do(ctx, "private_table_key", func(context.Context) error { t.Fatal("backoff called fill"); return nil }); !errors.Is(err, ErrBackoff) {
		t.Fatal(err)
	}
	operations = map[string]int{}
	for _, operation := range trace.Snapshot().Operations {
		operations[operation.Name] = operation.Count
	}
	if operations["masterdata.cachefill_wait"] != 1 || operations["masterdata.cachefill_backoff"] != 1 {
		t.Fatalf("operations=%v", operations)
	}
	raw, err = json.Marshal(trace.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private_table_key", "private_failure_value"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("trace leaked value: %s", raw)
		}
	}
}
