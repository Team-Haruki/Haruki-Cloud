package cachepersist

import (
	"context"
	"errors"
	"testing"
)

func TestTasksDrainCancelsAndRejectsNewWork(t *testing.T) {
	var tasks Tasks
	life, stop := context.WithCancel(t.Context())
	defer stop()
	if err := tasks.Configure(life); err != nil {
		t.Fatal(err)
	}
	active, finish, err := tasks.Start()
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := tasks.Close(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("close=%v", err)
	}
	if !errors.Is(active.Err(), context.Canceled) {
		t.Fatal("task was not canceled")
	}
	if _, _, err := tasks.Start(); err == nil {
		t.Fatal("accepted work after shutdown")
	}
	finish()
	finish()
	if err := tasks.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
