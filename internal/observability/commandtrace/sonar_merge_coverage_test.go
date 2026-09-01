package commandtrace

import (
	"testing"
	"time"
)

func TestTraceMergeInitializesBothTargetsAndSkipsInvalidStats(t *testing.T) {
	(*Trace)(nil).merge([]Stats{{Name: "ignored", Count: 1}}, false)
	trace := &Trace{}
	trace.merge(nil, false)
	trace.merge([]Stats{
		{Name: " ", Count: 1},
		{Name: "ignored", Count: 0},
		{Name: "operation", Count: 2, Total: 3 * time.Millisecond, Max: 2 * time.Millisecond},
	}, false)
	trace.merge([]Stats{{Name: "phase", Count: 1, Total: time.Millisecond, Max: time.Millisecond}}, true)
	snapshot := trace.Snapshot()
	if len(snapshot.Operations) != 1 || snapshot.Operations[0].Count != 2 {
		t.Fatalf("operations = %#v", snapshot.Operations)
	}
	if len(snapshot.Phases) != 1 || snapshot.Phases[0].Name != "phase" {
		t.Fatalf("phases = %#v", snapshot.Phases)
	}
}
