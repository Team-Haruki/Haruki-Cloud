package authban

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"haruki-cloud/config"
	"haruki-cloud/internal/core/secevent"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type captureReporter struct {
	mu     sync.Mutex
	events []secevent.Event
}

func (r *captureReporter) Report(_ context.Context, ev secevent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *captureReporter) all() []secevent.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]secevent.Event(nil), r.events...)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type testEnv struct {
	mr       *miniredis.Miniredis
	rdb      *redis.Client
	guard    *Guard
	reporter *captureReporter
	logs     *syncBuffer
	clock    time.Time
}

// advance moves both Redis TTLs and the guard's clock.
func (e *testEnv) advance(d time.Duration) {
	e.mr.FastForward(d)
	e.clock = e.clock.Add(d)
}

func newTestEnv(t *testing.T, cfg Config) *testEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	reporter := &captureReporter{}
	guard, err := New(cfg, rdb, reporter)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env := &testEnv{mr: mr, rdb: rdb, guard: guard, reporter: reporter, logs: &syncBuffer{}, clock: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	guard.logger = slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	guard.now = func() time.Time { return env.clock }
	return env
}

const (
	attackerIP = "203.0.113.50"
	botA       = "76313098"
	botB       = "30042042"
)

func (e *testEnv) fail(t *testing.T, ip, bot string, n int) {
	t.Helper()
	for range n {
		e.guard.RecordFailure(context.Background(), ip, bot, ReasonAuthFailed)
	}
}

func TestDefaultsApply(t *testing.T) {
	cfg, enabled := ConfigFromSettings(config.AuthIPBanConfig{})
	if !enabled || !cfg.CountBuildRejected || !cfg.ExemptKnownBots || cfg.BlockBotRoutes {
		t.Fatalf("settings defaults = %+v enabled=%v", cfg, enabled)
	}
	eff := cfg.withDefaults()
	if eff.Threshold != 10 || eff.Window != 10*time.Minute || eff.BanDuration != 6*time.Hour ||
		eff.MaxBanDuration != 24*time.Hour || eff.EscalationWindow != 7*24*time.Hour || eff.KnownBotTTL != 7*24*time.Hour {
		t.Fatalf("defaults = %+v", eff)
	}
	off := false
	if _, enabled := ConfigFromSettings(config.AuthIPBanConfig{Enabled: &off}); enabled {
		t.Fatal("enabled: false ignored")
	}
	// A cap below the first ban means "no escalation", never a shorter ban.
	if got := (Config{BanDuration: time.Hour, MaxBanDuration: time.Minute}).withDefaults().MaxBanDuration; got != time.Hour {
		t.Fatalf("max below base = %v, want base", got)
	}
}

func TestThresholdEdge(t *testing.T) {
	env := newTestEnv(t, Config{})
	ctx := context.Background()

	env.fail(t, attackerIP, botA, DefaultThreshold-1)
	if d := env.guard.Check(ctx, attackerIP, botA); d.Banned {
		t.Fatal("banned one failure below the threshold")
	}
	if len(env.reporter.all()) != 0 {
		t.Fatal("event reported before the ban")
	}

	env.fail(t, attackerIP, botA, 1)
	d := env.guard.Check(ctx, attackerIP, botA)
	if !d.Banned || d.RetryAfter <= 6*time.Hour-time.Second || d.RetryAfter > 6*time.Hour {
		t.Fatalf("after threshold: %+v, want banned ~6h", d)
	}
	// The window state is consumed by the ban.
	if env.mr.Exists(keysFor(attackerIP).fail) || env.mr.Exists(keysFor(attackerIP).bots) {
		t.Fatal("window keys survived the ban")
	}

	// Failures while banned (an exempt bot that then fails) are not counted
	// and never ban twice.
	env.fail(t, attackerIP, botA, 25)
	if got := len(env.reporter.all()); got != 1 {
		t.Fatalf("events = %d, want exactly one ban", got)
	}
	ev := env.reporter.all()[0]
	if ev.Kind != secevent.KindIPBanned || ev.SourceIP != attackerIP || !ev.Enforced || ev.BotID != "" || ev.Alert == nil {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Alert.Count != 10 || ev.Alert.Threshold != 10 || ev.Alert.Window != 10*time.Minute ||
		ev.Alert.BanDuration != 6*time.Hour || len(ev.Alert.BotIDs) != 1 || ev.Alert.BotIDs[0] != botA {
		t.Fatalf("alert detail = %+v", ev.Alert)
	}
	if !strings.Contains(ev.Reason, "10 login failures in 10m0s") || !strings.Contains(ev.Reason, "banned for 6h0m0s") || !strings.Contains(ev.Reason, "bots "+botA) {
		t.Fatalf("reason = %q", ev.Reason)
	}
	if logs := env.logs.String(); !strings.Contains(logs, "auth ip ban started") || !strings.Contains(logs, "until=2026-10-10T18:00:00Z") {
		t.Fatalf("ban start log missing:\n%s", logs)
	}

	// Another address is unaffected.
	if d := env.guard.Check(ctx, "198.51.100.1", botA); d.Banned {
		t.Fatal("ban leaked to another address")
	}
}

func TestThresholdOneBansOnFirstFailure(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1})
	env.fail(t, attackerIP, "", 1)
	if !env.guard.Check(context.Background(), attackerIP, "").Banned {
		t.Fatal("threshold 1 did not ban")
	}
	if ev := env.reporter.all(); len(ev) != 1 || len(ev[0].Alert.BotIDs) != 0 || strings.Contains(ev[0].Reason, "bots") {
		t.Fatalf("event without bots = %+v", ev)
	}
}

func TestWindowExpiryRestartsCount(t *testing.T) {
	env := newTestEnv(t, Config{})
	ctx := context.Background()
	env.fail(t, attackerIP, botA, 9)
	if ttl := env.mr.TTL(keysFor(attackerIP).fail); ttl != 10*time.Minute {
		t.Fatalf("window ttl = %v, want 10m from the first failure", ttl)
	}
	if ttl := env.mr.TTL(keysFor(attackerIP).bots); ttl <= 0 || ttl > 10*time.Minute {
		t.Fatalf("bots ttl = %v, want the window's", ttl)
	}
	env.advance(10*time.Minute + time.Second)
	env.fail(t, attackerIP, botA, 9)
	if env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("failures from an expired window were still counted")
	}
	env.fail(t, attackerIP, botA, 1)
	if !env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("tenth failure in the new window did not ban")
	}
}

func TestBanExpiryAndSweep(t *testing.T) {
	env := newTestEnv(t, Config{})
	ctx := context.Background()
	env.fail(t, attackerIP, botA, 10)

	env.advance(6*time.Hour - time.Minute)
	if !env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("ban ended early")
	}
	env.guard.SweepExpired(ctx)
	if strings.Contains(env.logs.String(), "auth ip ban expired") {
		t.Fatal("expiry logged before the ban ended")
	}

	env.advance(time.Minute)
	if env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("ban outlived its duration")
	}
	env.guard.SweepExpired(ctx)
	env.guard.SweepExpired(ctx) // a second sweep (another instance) must not log again
	if n := strings.Count(env.logs.String(), "auth ip ban expired"); n != 1 {
		t.Fatalf("expiry logged %d times, want 1:\n%s", n, env.logs.String())
	}
	if bans, err := env.guard.List(ctx); err != nil || len(bans) != 0 {
		t.Fatalf("list after expiry = %+v, %v", bans, err)
	}
}

func TestEscalationDoublesUpToCapAndResets(t *testing.T) {
	env := newTestEnv(t, Config{})
	ctx := context.Background()
	var got []time.Duration
	for range 4 {
		env.fail(t, attackerIP, botA, 10)
		evs := env.reporter.all()
		dur := evs[len(evs)-1].Alert.BanDuration
		got = append(got, dur)
		env.advance(dur)
	}
	want := []time.Duration{6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ban lengths = %v, want %v", got, want)
		}
	}
	// Seven days after the last ban the address starts over.
	env.advance(7 * 24 * time.Hour)
	env.fail(t, attackerIP, botA, 10)
	evs := env.reporter.all()
	if last := evs[len(evs)-1].Alert.BanDuration; last != 6*time.Hour {
		t.Fatalf("after the escalation window: %v, want 6h", last)
	}
	st, err := env.guard.Inspect(ctx, attackerIP)
	if err != nil || st.Ban == nil || st.Ban.Level != 1 {
		t.Fatalf("inspect = %+v, %v", st, err)
	}
}

func TestEscalationDisabledWhenCapEqualsBase(t *testing.T) {
	env := newTestEnv(t, Config{BanDuration: time.Hour, MaxBanDuration: time.Hour})
	for range 3 {
		env.fail(t, attackerIP, "", 10)
		env.advance(time.Hour)
	}
	for _, ev := range env.reporter.all() {
		if ev.Alert.BanDuration != time.Hour {
			t.Fatalf("escalated without room: %v", ev.Alert.BanDuration)
		}
	}
}

func TestNeverBanRanges(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1, NeverBan: []string{"198.51.100.0/24", "192.0.2.7", " "}})
	ctx := context.Background()
	for _, ip := range []string{
		"127.0.0.1", "10.1.2.3", "172.20.0.5", "192.168.1.1", "100.100.1.1", "::1", "fd00::1",
		"fe80::1%eth0", "::ffff:10.0.0.1", "198.51.100.9", "192.0.2.7", "", "not-an-ip",
	} {
		env.guard.RecordFailure(ctx, ip, botA, ReasonAuthFailed)
		if env.guard.Check(ctx, ip, botA).Banned {
			t.Fatalf("%q was banned", ip)
		}
	}
	if keys := env.mr.Keys(); len(keys) != 0 {
		t.Fatalf("never-ban addresses touched Redis: %v", keys)
	}
	st, err := env.guard.Inspect(ctx, "100.64.0.1")
	if err != nil || !st.NeverBanned {
		t.Fatalf("inspect never-ban = %+v, %v", st, err)
	}
	// The next address outside the operator's /24 still counts.
	env.guard.RecordFailure(ctx, "198.51.101.1", botA, ReasonAuthFailed)
	if !env.guard.Check(ctx, "198.51.101.1", botA).Banned {
		t.Fatal("address outside the allowlist was not banned")
	}
}

func TestInvalidNeverBanEntry(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	defer rdb.Close()
	for _, bad := range []string{"10.0.0.0/33", "nope", "1.2.3"} {
		if _, err := New(Config{NeverBan: []string{bad}}, rdb, nil); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if _, err := New(Config{}, nil, nil); err == nil {
		t.Fatal("nil redis accepted")
	}
	g, err := New(Config{NeverBan: []string{"::ffff:203.0.113.0/120"}}, rdb, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g.subject("203.0.113.9"); ok {
		t.Fatal("v4-mapped never-ban prefix not unmapped")
	}
}

func TestIPv6CountsPerSlash64(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 3})
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		env.guard.RecordFailure(ctx, "2001:db8:1:2::"+strconv.Itoa(i), botA, ReasonAuthFailed)
	}
	if !env.guard.Check(ctx, "2001:db8:1:2:ffff::1", botA).Banned {
		t.Fatal("rotating inside one /64 escaped the ban")
	}
	if env.guard.Check(ctx, "2001:db8:1:3::1", botA).Banned {
		t.Fatal("neighbouring /64 was banned")
	}
	bans, err := env.guard.List(ctx)
	if err != nil || len(bans) != 1 || bans[0].IP != "2001:db8:1:2::/64" {
		t.Fatalf("list = %+v, %v", bans, err)
	}
}

func TestFailureKinds(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cfg        Config
		reason     Reason
		wantBanned bool
	}{
		{"auth failed", Config{}, ReasonAuthFailed, true},
		{"replay", Config{}, ReasonReplay, true},
		{"noise", Config{}, ReasonHandshake, true},
		{"build rejected counted", Config{CountBuildRejected: true}, ReasonBuildRejected, true},
		{"build rejected ignored", Config{CountBuildRejected: false}, ReasonBuildRejected, false},
		{"unknown", Config{}, Reason("rate_limited"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.Threshold = 2
			env := newTestEnv(t, cfg)
			for range 2 {
				env.guard.RecordFailure(context.Background(), attackerIP, botA, tc.reason)
			}
			if got := env.guard.Check(context.Background(), attackerIP, botA).Banned; got != tc.wantBanned {
				t.Fatalf("banned = %v, want %v", got, tc.wantBanned)
			}
		})
	}
}

func TestKnownBotsStayAdmitted(t *testing.T) {
	env := newTestEnv(t, Config{ExemptKnownBots: true})
	ctx := context.Background()
	env.guard.RecordSuccess(ctx, attackerIP, botB)
	if ttl := env.mr.TTL(keysFor(attackerIP).known); ttl != DefaultKnownBotTTL {
		t.Fatalf("known ttl = %v", ttl)
	}
	// A success never resets the address's failures.
	env.fail(t, attackerIP, botA, 9)
	env.guard.RecordSuccess(ctx, attackerIP, botB)
	env.fail(t, attackerIP, botA, 1)

	if !env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("failing bot not banned")
	}
	if env.guard.Check(ctx, attackerIP, botB).Banned {
		t.Fatal("bot that logged in from this address was locked out")
	}
	if !env.guard.Check(ctx, attackerIP, "").Banned || !env.guard.Check(ctx, attackerIP, "x1").Banned {
		t.Fatal("request without a valid bot id was admitted")
	}
	// Once the known bot fails it loses the exemption.
	env.guard.RecordFailure(ctx, attackerIP, botB, ReasonAuthFailed)
	if !env.guard.Check(ctx, attackerIP, botB).Banned {
		t.Fatal("known bot kept its exemption after failing")
	}

	st, err := env.guard.Inspect(ctx, attackerIP)
	if err != nil || len(st.KnownBotIDs) != 0 || st.Ban == nil {
		t.Fatalf("inspect = %+v, %v", st, err)
	}
}

func TestKnownBotsIgnoredWhenExemptionOff(t *testing.T) {
	env := newTestEnv(t, Config{ExemptKnownBots: false})
	ctx := context.Background()
	env.guard.RecordSuccess(ctx, attackerIP, botB)
	if env.mr.Exists(keysFor(attackerIP).known) {
		t.Fatal("known bot stored with the exemption off")
	}
	env.fail(t, attackerIP, botA, 10)
	if !env.guard.Check(ctx, attackerIP, botB).Banned {
		t.Fatal("exemption applied while off")
	}
	env.guard.RecordSuccess(ctx, "10.0.0.1", botB)
	g := env.guard
	g.cfg.ExemptKnownBots = true
	g.RecordSuccess(ctx, "10.0.0.1", botB)
	g.RecordSuccess(ctx, attackerIP, "bad")
	if env.mr.Exists(keysFor("10.0.0.1").known) || env.mr.Exists(keysFor(attackerIP).known) {
		t.Fatal("never-ban address or invalid bot id stored")
	}
}

func TestConcurrentFailuresBanExactlyOnce(t *testing.T) {
	env := newTestEnv(t, Config{})
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			env.guard.RecordFailure(context.Background(), attackerIP, strconv.Itoa(1000+i%30), ReasonAuthFailed)
		}()
	}
	wg.Wait()
	evs := env.reporter.all()
	if len(evs) != 1 {
		t.Fatalf("bans = %d, want 1", len(evs))
	}
	if evs[0].Alert.Count != 10 {
		t.Fatalf("ban count = %d, want exactly the threshold", evs[0].Alert.Count)
	}
	if n := len(evs[0].Alert.BotIDs); n == 0 || n > maxBotsPerWindow {
		t.Fatalf("bots = %d", n)
	}
}

func TestBotListIsBounded(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 50})
	for i := range 30 {
		env.guard.RecordFailure(context.Background(), attackerIP, strconv.Itoa(5000+i), ReasonAuthFailed)
	}
	if n, _ := env.rdb.SCard(context.Background(), keysFor(attackerIP).bots).Result(); n != maxBotsPerWindow {
		t.Fatalf("bots stored = %d, want %d", n, maxBotsPerWindow)
	}
	// Junk bot ids are counted but not stored.
	env.guard.RecordFailure(context.Background(), attackerIP, strings.Repeat("9", 21), ReasonAuthFailed)
	st, _ := env.guard.Inspect(context.Background(), attackerIP)
	if st.Failures != 31 {
		t.Fatalf("failures = %d", st.Failures)
	}
}

func TestAdminListInspectUnban(t *testing.T) {
	env := newTestEnv(t, Config{})
	ctx := context.Background()
	env.fail(t, attackerIP, botA, 10)
	env.advance(time.Hour)
	env.fail(t, "198.51.100.20", botB, 10)
	env.fail(t, "192.0.2.200", botB, 3)

	bans, err := env.guard.List(ctx)
	if err != nil || len(bans) != 2 {
		t.Fatalf("list = %+v, %v", bans, err)
	}
	first := bans[0]
	if first.IP != attackerIP || first.Level != 1 || first.Failures != 10 || len(first.BotIDs) != 1 || first.BotIDs[0] != botA ||
		first.RetryAfterSeconds != 5*3600 || !first.Until.Equal(first.Since.Add(6*time.Hour)) {
		t.Fatalf("first ban = %+v", first)
	}

	st, err := env.guard.Inspect(ctx, "192.0.2.200")
	if err != nil || st.Ban != nil || st.Failures != 3 || st.WindowRemainingSeconds != 600 || len(st.BotIDs) != 1 || st.Level != 0 {
		t.Fatalf("inspect counting = %+v, %v", st, err)
	}

	subject, was, err := env.guard.Unban(ctx, attackerIP)
	if err != nil || !was || subject != attackerIP {
		t.Fatalf("unban = %q %v %v", subject, was, err)
	}
	if env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("still banned after unban")
	}
	st, _ = env.guard.Inspect(ctx, attackerIP)
	if st.Level != 0 || st.Failures != 0 || st.Ban != nil {
		t.Fatalf("counters survived unban: %+v", st)
	}
	if _, was, _ := env.guard.Unban(ctx, attackerIP); was {
		t.Fatal("second unban reported a ban")
	}
	if !strings.Contains(env.logs.String(), "auth ip ban lifted") {
		t.Fatal("unban not logged")
	}
	// The next ban after an unban starts from the first length again.
	env.fail(t, attackerIP, botA, 10)
	evs := env.reporter.all()
	if evs[len(evs)-1].Alert.BanDuration != 6*time.Hour {
		t.Fatal("escalation level survived unban")
	}

	if _, err := env.guard.Inspect(ctx, "nope"); err == nil {
		t.Fatal("invalid inspect accepted")
	}
	if _, _, err := env.guard.Unban(ctx, "2001:db8::/48"); err == nil {
		t.Fatal("non-/64 prefix accepted")
	}
}

func TestNormalizeSubject(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.5":          "203.0.113.5",
		" ::ffff:203.0.113.5 ": "203.0.113.5",
		"2001:db8::1":          "2001:db8::/64",
		"2001:db8:0:0:1::/64":  "2001:db8::/64",
	} {
		got, err := NormalizeSubject(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeSubject(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "1.2.3.0/24", "::ffff:1.2.3.0/120", "x"} {
		if _, err := NormalizeSubject(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestRedisFailureFailsOpen(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1})
	ctx := context.Background()
	env.fail(t, attackerIP, botA, 1)
	env.guard.RecordSuccess(ctx, attackerIP, botB)
	before := metrics.Get(metricRedisErrors)
	env.mr.Close()

	if env.guard.Check(ctx, attackerIP, botA).Banned {
		t.Fatal("redis outage must fail open")
	}
	env.guard.RecordFailure(ctx, attackerIP, botA, ReasonAuthFailed)
	env.guard.RecordSuccess(ctx, attackerIP, botB)
	env.guard.SweepExpired(ctx)
	if _, err := env.guard.List(ctx); err == nil {
		t.Fatal("list hid the outage")
	}
	if _, err := env.guard.Inspect(ctx, attackerIP); err == nil {
		t.Fatal("inspect hid the outage")
	}
	if _, _, err := env.guard.Unban(ctx, attackerIP); err == nil {
		t.Fatal("unban hid the outage")
	}
	if after := metrics.Get(metricRedisErrors); after == nil || before != nil && after.String() == before.String() {
		t.Fatal("redis errors not counted")
	}
	if !strings.Contains(env.logs.String(), "auth ip ban store unavailable") {
		t.Fatal("outage not logged")
	}
}

func TestKnownLookupFailureFailsOpen(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1, ExemptKnownBots: true})
	env.fail(t, attackerIP, botA, 1)
	// A wrong-type known key makes SISMEMBER fail while the ban key is fine.
	env.mr.Set(keysFor(attackerIP).known, "x")
	if env.guard.Check(context.Background(), attackerIP, botB).Banned {
		t.Fatal("known lookup failure must fail open")
	}
}

func TestNilGuardIsDisabled(t *testing.T) {
	var g *Guard
	ctx := context.Background()
	if g.Check(ctx, attackerIP, botA).Banned || g.BlocksBotRoutes() || g.Config().Threshold != 0 {
		t.Fatal("nil guard not disabled")
	}
	g.RecordFailure(ctx, attackerIP, botA, ReasonAuthFailed)
	g.RecordSuccess(ctx, attackerIP, botA)
	g.SweepExpired(ctx)
	g.RunExpirySweeper(ctx, time.Second)
	if bans, err := g.List(ctx); err != nil || len(bans) != 0 {
		t.Fatal("nil list")
	}
	if st, err := g.Inspect(ctx, attackerIP); err != nil || st.Ban != nil {
		t.Fatal("nil inspect")
	}
	if _, was, err := g.Unban(ctx, attackerIP); err != nil || was {
		t.Fatal("nil unban")
	}
}

func TestRunExpirySweeperStopsWithContext(t *testing.T) {
	env := newTestEnv(t, Config{Threshold: 1, BanDuration: time.Minute})
	env.fail(t, attackerIP, botA, 1)
	env.advance(2 * time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		env.guard.RunExpirySweeper(ctx, 5*time.Millisecond)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(env.logs.String(), "auth ip ban expired") {
		if time.Now().After(deadline) {
			t.Fatal("sweeper never logged the expiry")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper ignored cancellation")
	}
	env.guard.RunExpirySweeper(context.Background(), 0) // returns at once
}
