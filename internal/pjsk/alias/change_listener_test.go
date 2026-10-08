package alias

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

type recordedChanges struct {
	mu      sync.Mutex
	changes []AliasChange
}

func (r *recordedChanges) listener() ChangeListener {
	return func(_ context.Context, changes []AliasChange) {
		r.mu.Lock()
		r.changes = append(r.changes, changes...)
		r.mu.Unlock()
	}
}

func TestApproveAndDeleteNotifyChangeListener(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	var recorded recordedChanges
	deps.service.SetChangeListener(recorded.listener())

	deps.addMusic(t, ctx, 5301, "群青讃歌")
	deps.addAdmin(t, ctx, "qq", "9101", "Alias Admin")
	records, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "42", "5301", []string{"蓝歌"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if len(recorded.changes) != 0 {
		t.Fatalf("submission must not notify, got %+v", recorded.changes)
	}
	if _, err := deps.service.Approve(ctx, "qq", "9101", []int64{records[0].ReviewID}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	want := []AliasChange{{AliasType: PjskAliasTypeMusic, AliasTypeID: 5301, Alias: "蓝歌"}}
	if !reflect.DeepEqual(recorded.changes, want) {
		t.Fatalf("approve changes = %+v, want %+v", recorded.changes, want)
	}

	recorded.changes = nil
	if _, err := deps.service.Delete(ctx, PjskAliasTypeMusic, "qq", "9101", "5301", []string{"蓝歌"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !reflect.DeepEqual(recorded.changes, want) {
		t.Fatalf("delete changes = %+v, want %+v", recorded.changes, want)
	}

	recorded.changes = nil
	if _, err := deps.service.Delete(ctx, PjskAliasTypeMusic, "qq", "9101", "5301", []string{"蓝歌"}); err == nil {
		t.Fatal("deleting a missing alias should fail")
	}
	if len(recorded.changes) != 0 {
		t.Fatalf("a failed delete must not notify, got %+v", recorded.changes)
	}
}

func TestChangeListenerNilSafe(t *testing.T) {
	var s *Service
	s.SetChangeListener(func(context.Context, []AliasChange) {})
	s.notifyChanged(context.Background(), []AliasChange{{AliasType: "music"}})
	(&Service{}).notifyChanged(context.Background(), []AliasChange{{AliasType: "music"}})
}
