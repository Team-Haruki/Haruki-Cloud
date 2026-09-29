package mysekai

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type snapshotTimerBytes struct {
	snapshot.Snapshot
	data []byte
	err  error
}

func (s snapshotTimerBytes) RawBytes() ([]byte, error) { return append([]byte(nil), s.data...), s.err }

func TestMysekaiSnapshotTimers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       []byte
		snap      snapshot.Snapshot
		wantError bool
		want      map[string]int
	}{
		{"direct", []byte(`{"updatedResources":{"private_field":"private_value"}}`), nil, false, map[string]int{"mysekai.snapshot_decode": 1, "mysekai.snapshot_flatten": 1}},
		{"suite", nil, snapshotTimerBytes{data: []byte(`{"private_field":"private_value"}`)}, false, map[string]int{"mysekai.snapshot_copy": 1, "mysekai.snapshot_decode": 1}},
		{"copy_failure", nil, snapshotTimerBytes{err: context.Canceled}, true, map[string]int{"mysekai.snapshot_copy": 1}},
		{"decode_failure", []byte(`{"private_field":`), nil, true, map[string]int{"mysekai.snapshot_decode": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(t.Context())
			controller := &Controller{requestCtx: ctx, rawMySekaiJSON: tc.raw, snapshot: tc.snap}
			data, _, err := controller.decodeSnapshot("jp")
			if (err != nil) != tc.wantError {
				t.Fatalf("decode error=%v", err)
			}
			if tc.name == "copy_failure" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if !tc.wantError && data["private_field"] != "private_value" {
				t.Fatal("decoded payload changed")
			}
			counts := map[string]int{}
			for _, operation := range trace.Snapshot().Operations {
				counts[operation.Name] = operation.Count
			}
			if !reflect.DeepEqual(counts, tc.want) {
				t.Fatalf("operations=%v want=%v", counts, tc.want)
			}
			raw, err := json.Marshal(trace.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "private_field") || strings.Contains(string(raw), "private_value") {
				t.Fatalf("trace leaked data: %s", raw)
			}
		})
	}
}
