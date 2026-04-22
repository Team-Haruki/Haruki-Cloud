package upstream

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResolveTargetsFallsBackToLegacyBaseURL(t *testing.T) {
	targets := ResolveTargets(" https://drawing.example.com/ ", nil, "drawing")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Name != "drawing-1" {
		t.Fatalf("unexpected target name: %q", targets[0].Name)
	}
	if targets[0].BaseURL != "https://drawing.example.com" {
		t.Fatalf("unexpected target base url: %q", targets[0].BaseURL)
	}
	if targets[0].Concurrency != 0 {
		t.Fatalf("legacy base_url should stay unlimited by default, got %d", targets[0].Concurrency)
	}
}

func TestResolveTargetsPrefersExplicitTargets(t *testing.T) {
	targets := ResolveTargets("https://legacy.example.com", []TargetConfig{
		{BaseURL: " https://one.example.com/ "},
		{Name: "deck-b", BaseURL: "https://two.example.com", Concurrency: 4},
		{BaseURL: "   "},
	}, "deck-service")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Name != "deck-service-1" {
		t.Fatalf("unexpected auto target name: %q", targets[0].Name)
	}
	if targets[0].BaseURL != "https://one.example.com" {
		t.Fatalf("unexpected auto target base url: %q", targets[0].BaseURL)
	}
	if targets[1].Name != "deck-b" || targets[1].Concurrency != 4 {
		t.Fatalf("unexpected explicit target config: %+v", targets[1])
	}
}

func TestPoolAcquirePrefersLowerPendingTarget(t *testing.T) {
	pool := NewPool([]TargetConfig{
		{Name: "a", BaseURL: "https://a.example.com", Concurrency: 1},
		{Name: "b", BaseURL: "https://b.example.com", Concurrency: 1},
	})

	first, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer first.Release()

	second, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	defer second.Release()

	if first.Target.BaseURL == second.Target.BaseURL {
		t.Fatalf("expected second acquire to choose the less-pending target, got %q twice", first.Target.BaseURL)
	}
}

func TestPoolAcquireHonorsContextWhenTargetQueueIsFull(t *testing.T) {
	pool := NewPool([]TargetConfig{
		{Name: "only", BaseURL: "https://only.example.com", Concurrency: 1},
	})

	lease, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("initial acquire failed: %v", err)
	}
	defer lease.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = pool.Acquire(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestPoolAcquireAllowsUnlimitedTargetsWhenConcurrencyIsZero(t *testing.T) {
	pool := NewPool([]TargetConfig{
		{Name: "only", BaseURL: "https://only.example.com"},
	})

	first, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer first.Release()

	second, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	defer second.Release()

	if first.Target.BaseURL != second.Target.BaseURL {
		t.Fatalf("expected same unlimited target, got %q and %q", first.Target.BaseURL, second.Target.BaseURL)
	}
}

func TestPoolsWithSameTargetNameShareConcurrencyBudget(t *testing.T) {
	shared := &SharedResources{}
	drawingPool := NewPoolWithResources([]TargetConfig{
		{Name: "vm100", BaseURL: "http://drawing:8000", Concurrency: 1},
	}, shared)
	deckPool := NewPoolWithResources([]TargetConfig{
		{Name: "vm100", BaseURL: "http://deck-service:3000", Concurrency: 1},
	}, shared)

	first, err := drawingPool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("drawing acquire failed: %v", err)
	}
	defer first.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = deckPool.Acquire(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shared queue to block second service, got %v", err)
	}
}

func TestPoolsWithDifferentTargetNamesDoNotShareConcurrencyBudget(t *testing.T) {
	shared := &SharedResources{}
	drawingPool := NewPoolWithResources([]TargetConfig{
		{Name: "vm100-drawing", BaseURL: "http://drawing:8000", Concurrency: 1},
	}, shared)
	deckPool := NewPoolWithResources([]TargetConfig{
		{Name: "vm100-deck", BaseURL: "http://deck-service:3000", Concurrency: 1},
	}, shared)

	first, err := drawingPool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("drawing acquire failed: %v", err)
	}
	defer first.Release()

	second, err := deckPool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("deck acquire should not be blocked: %v", err)
	}
	defer second.Release()
}
