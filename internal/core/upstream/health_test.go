package upstream

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"haruki-cloud/internal/testutil"
)

type healthEvent struct {
	name string
	down bool
}

func TestHealthWindowAndTransitions(t *testing.T) {
	clock := testutil.NewFakeClock()
	var events []healthEvent
	health := NewHealth(30*time.Second, clock.Now, func(name string, down bool) {
		events = append(events, healthEvent{name, down})
	})

	testutil.Require(t, health.Healthy("a"), "unknown target must be healthy")
	health.MarkUp("a")
	testutil.Require(t, len(events) == 0, "MarkUp of a healthy target must not report, got %v", events)

	health.MarkDown("a")
	health.MarkDown("a")
	testutil.Require(t, !health.Healthy("a"), "target must be down inside the window")
	testutil.Require(t, slices.Equal(events, []healthEvent{{"a", true}}), "repeated MarkDown must report once, got %v", events)

	clock.Advance(29 * time.Second)
	testutil.Require(t, !health.Healthy("a"), "target must stay down until the window ends")
	clock.Advance(time.Second)
	testutil.Require(t, health.Healthy("a"), "target must be eligible once the window has passed")

	health.MarkDown("a")
	testutil.Require(t, len(events) == 2 && events[1] == (healthEvent{"a", true}), "a failure after the window must report again, got %v", events)
	health.MarkUp("a")
	testutil.Require(t, health.Healthy("a"), "MarkUp must clear the down state at once")
	testutil.Require(t, len(events) == 3 && events[2] == (healthEvent{"a", false}), "MarkUp of a down target must report, got %v", events)

	var nilHealth *Health
	nilHealth.MarkDown("a")
	nilHealth.MarkUp("a")
	testutil.Require(t, nilHealth.Healthy("a"), "nil Health reports every target healthy")
}

func TestAcquireHealthyTiers(t *testing.T) {
	targets := []TargetConfig{
		{Name: "a", BaseURL: "https://a.example.com"},
		{Name: "b", BaseURL: "https://b.example.com"},
		{Name: "standby", BaseURL: "https://standby.example.com", Backup: true},
	}
	cases := []struct {
		name    string
		down    []string
		exclude []string
		want    []string // acceptable picks
	}{
		{name: "all healthy uses primaries", want: []string{"a", "b"}},
		{name: "one primary down", down: []string{"a"}, want: []string{"b"}},
		{name: "primaries down uses backup", down: []string{"a", "b"}, want: []string{"standby"}},
		{name: "primaries tried uses backup", exclude: []string{"a", "b"}, want: []string{"standby"}},
		{name: "all down prefers primaries", down: []string{"a", "b", "standby"}, want: []string{"a", "b"}},
		{name: "healthy backup beats down primary", down: []string{"a"}, exclude: []string{"b"}, want: []string{"standby"}},
		{name: "down primary beats down backup", down: []string{"a", "standby"}, exclude: []string{"b"}, want: []string{"a"}},
		{name: "only down backup left", down: []string{"standby"}, exclude: []string{"a", "b"}, want: []string{"standby"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := NewPool(targets)
			health := NewHealth(time.Minute, nil, nil)
			for _, name := range tc.down {
				health.MarkDown(name)
			}
			for range 6 {
				lease, err := pool.AcquireHealthy(context.Background(), health, tc.exclude)
				testutil.Require(t, err == nil, "AcquireHealthy: %v", err)
				testutil.Check(t, slices.Contains(tc.want, lease.Target.Name), "picked %q, want one of %v", lease.Target.Name, tc.want)
				lease.Release()
			}
		})
	}

	pool := NewPool(targets)
	_, err := pool.AcquireHealthy(context.Background(), nil, []string{"a", "b", "standby"})
	testutil.Require(t, errors.Is(err, ErrNoAvailableTargets), "every target excluded must fail, got %v", err)
	_, err = (*Pool)(nil).AcquireHealthy(context.Background(), nil, nil)
	testutil.Require(t, errors.Is(err, ErrNoTargetsConfigured), "nil pool must report no targets, got %v", err)
}

func TestAcquireHealthyRoundRobinsPrimariesOnly(t *testing.T) {
	pool := NewPool([]TargetConfig{
		{Name: "a", BaseURL: "https://a.example.com"},
		{Name: "standby", BaseURL: "https://standby.example.com", Backup: true},
		{Name: "b", BaseURL: "https://b.example.com"},
	})
	var picks []string
	for range 6 {
		lease, err := pool.AcquireHealthy(context.Background(), nil, nil)
		testutil.Require(t, err == nil, "AcquireHealthy: %v", err)
		picks = append(picks, lease.Target.Name)
		lease.Release()
	}
	for i := 1; i < len(picks); i++ {
		testutil.Check(t, picks[i] != picks[i-1] && picks[i] != "standby", "idle picks must alternate between primaries, got %v", picks)
	}
}

func TestAcquireHealthyPrefersLessPendingPrimary(t *testing.T) {
	pool := NewPool([]TargetConfig{
		{Name: "a", BaseURL: "https://a.example.com", Concurrency: 1},
		{Name: "b", BaseURL: "https://b.example.com", Concurrency: 1},
		{Name: "standby", BaseURL: "https://standby.example.com", Backup: true},
	})
	first, err := pool.AcquireHealthy(context.Background(), nil, nil)
	testutil.Require(t, err == nil, "first acquire: %v", err)
	defer first.Release()
	second, err := pool.AcquireHealthy(context.Background(), nil, nil)
	testutil.Require(t, err == nil, "second acquire: %v", err)
	defer second.Release()
	testutil.Require(t, first.Target.Name != second.Target.Name, "second lease must go to the idle primary, got %q twice", first.Target.Name)

	// Both primaries are saturated; the backup still gets nothing while they
	// are healthy, so the third caller waits for a primary slot.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = pool.AcquireHealthy(ctx, nil, nil)
	testutil.Require(t, errors.Is(err, context.DeadlineExceeded), "saturated healthy primaries must not spill to the backup, got %v", err)
}

func TestBackupFlagIsNeutralForOtherAcquires(t *testing.T) {
	targets := ResolveTargets("", []TargetConfig{
		{Name: "a", BaseURL: "https://a.example.com"},
		{Name: "standby", BaseURL: "https://standby.example.com/", Backup: true},
	}, "x")
	testutil.Require(t, len(targets) == 2 && targets[1].Backup, "ResolveTargets must keep backup, got %+v", targets)
	pool := NewPool(targets)
	seen := map[string]bool{}
	for range 4 {
		lease, err := pool.Acquire(context.Background())
		testutil.Require(t, err == nil, "Acquire: %v", err)
		seen[lease.Target.Name] = true
		testutil.Check(t, lease.Target.Backup == (lease.Target.Name == "standby"), "lease must carry the backup flag: %+v", lease.Target)
		lease.Release()
	}
	testutil.Require(t, seen["a"] && seen["standby"], "Acquire keeps plain rotation over every target, saw %v", seen)
}
