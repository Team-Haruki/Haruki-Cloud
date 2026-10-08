package upstream

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func affinityTargets() []TargetConfig {
	return []TargetConfig{
		{Name: "deck-a", BaseURL: "http://a", Concurrency: 1},
		{Name: "deck-b", BaseURL: "http://b", Concurrency: 1},
		{Name: "deck-c", BaseURL: "http://c", Concurrency: 1},
	}
}

func TestAcquireAffinityIsStablePerKey(t *testing.T) {
	pool := NewPool(affinityTargets())
	seen := make(map[string]int)
	for i := range 60 {
		key := []byte(fmt.Sprintf("digest-%d", i))
		first, err := pool.AcquireAffinity(context.Background(), key, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !first.Preferred {
			t.Fatalf("idle pool should give the preferred target for key %d", i)
		}
		name := first.Target.Name
		first.Release()
		for range 3 {
			again, err := pool.AcquireAffinity(context.Background(), key, nil)
			if err != nil {
				t.Fatal(err)
			}
			if again.Target.Name != name {
				t.Fatalf("key %d moved from %s to %s", i, name, again.Target.Name)
			}
			again.Release()
		}
		seen[name]++
	}
	if len(seen) != 3 {
		t.Fatalf("keys should spread over every target, got %v", seen)
	}
	if got := rendezvousScore([]byte("k"), "deck-a"); got != rendezvousScore([]byte("k"), "deck-a") {
		t.Fatal("rendezvous score must be deterministic")
	}
}

func TestAcquireAffinityFallsBackWhenPreferredIsFull(t *testing.T) {
	pool := NewPool(affinityTargets())
	key := []byte("busy-digest")
	held, err := pool.AcquireAffinity(context.Background(), key, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	fallback, err := pool.AcquireAffinity(ctx, key, nil)
	if err != nil {
		t.Fatalf("a full preferred target must not block: %v", err)
	}
	defer fallback.Release()
	if fallback.Preferred || fallback.Target.Name == held.Target.Name {
		t.Fatalf("expected a least-pending fallback, got %+v (held %s)", fallback, held.Target.Name)
	}
}

func TestAcquireAffinitySkipsRejectedTargets(t *testing.T) {
	pool := NewPool(affinityTargets())
	key := []byte("circuit-digest")
	preferred, err := pool.AcquireAffinity(context.Background(), key, nil)
	if err != nil {
		t.Fatal(err)
	}
	open := preferred.Target.Name
	preferred.Release()

	calls := make(map[string]int)
	lease, err := pool.AcquireAffinity(context.Background(), key, func(target TargetConfig) bool {
		calls[target.Name]++
		return target.Name != open
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if lease.Target.Name == open {
		t.Fatalf("circuit-open target %s was chosen", open)
	}
	if !lease.Preferred {
		t.Fatal("the next rendezvous target among accepted ones should be preferred")
	}
	for name, count := range calls {
		if count != 1 {
			t.Fatalf("accept called %d times for %s", count, name)
		}
	}

	if _, err := pool.AcquireAffinity(context.Background(), key, func(TargetConfig) bool { return false }); err != ErrNoAvailableTargets {
		t.Fatalf("no accepted target: err = %v", err)
	}
}

func TestAcquireAffinityEdgeCases(t *testing.T) {
	var nilPool *Pool
	if _, err := nilPool.AcquireAffinity(context.Background(), []byte("k"), nil); err != ErrNoTargetsConfigured {
		t.Fatalf("nil pool: %v", err)
	}
	pool := NewPool([]TargetConfig{{Name: "only", BaseURL: "http://x"}})
	lease, err := pool.AcquireAffinity(context.Background(), nil, nil)
	if err != nil || lease.Preferred {
		t.Fatalf("empty key should behave like AcquireFunc: %+v %v", lease, err)
	}
	lease.Release()
	var unset context.Context // a nil context is accepted like AcquireFunc does
	lease, err = pool.AcquireAffinity(unset, []byte("k"), nil)
	if err != nil || !lease.Preferred {
		t.Fatalf("unbounded target always has capacity: %+v %v", lease, err)
	}
	lease.Release()
}
