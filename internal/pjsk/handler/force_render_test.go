package handler

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestForceRenderLimiterCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newForceRenderLimiter()
	limiter.now = func() time.Time { return now }

	if !limiter.allow("u1", 0) {
		t.Fatal("first force denied")
	}
	if limiter.allow("u1", 0) {
		t.Fatal("second force inside the default cooldown allowed")
	}
	if !limiter.allow("u2", 0) {
		t.Fatal("another key shares the cooldown")
	}
	now = now.Add(defaultForceRenderCooldown)
	if !limiter.allow("u1", 0) {
		t.Fatal("force denied after the cooldown")
	}
	if !limiter.allow("u3", 5*time.Second) || limiter.allow("u3", 5*time.Second) {
		t.Fatal("configured cooldown not applied")
	}
	if limiter.allow("u4", -time.Second) {
		t.Fatal("negative cooldown must disable force")
	}
}

func TestForceRenderLimiterSweepsExpiredEntries(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newForceRenderLimiter()
	limiter.now = func() time.Time { return now }
	for i := range forceRenderLimiterSweepAt {
		limiter.next[string(rune(i))] = now.Add(-time.Second)
	}
	limiter.next["live"] = now.Add(time.Minute)
	if !limiter.allow("new", time.Minute) {
		t.Fatal("allow failed")
	}
	if len(limiter.next) != 2 {
		t.Fatalf("entries after sweep = %d, want live + new", len(limiter.next))
	}
}

func TestApplyForceRender(t *testing.T) {
	limiter := newForceRenderLimiter()
	base := context.Background()
	plain := &CommandRequest{RequesterPlatform: "qq", RequesterUserID: "1", CommandPath: "profile"}
	if drawing.ForceRenderFrom(applyForceRender(base, plain, limiter, 0)) {
		t.Fatal("request without --force was forced")
	}
	if drawing.ForceRenderFrom(applyForceRender(base, nil, limiter, 0)) {
		t.Fatal("nil request was forced")
	}
	anonymous := &CommandRequest{IsForce: true, CommandPath: "profile"}
	if drawing.ForceRenderFrom(applyForceRender(base, anonymous, limiter, 0)) {
		t.Fatal("force without a requester to rate-limit was honoured")
	}
	forced := &CommandRequest{IsForce: true, RequesterPlatform: "qq", RequesterUserID: "1", TriggerCommand: "/个人信息"}
	if !drawing.ForceRenderFrom(applyForceRender(base, forced, limiter, 0)) {
		t.Fatal("first --force not honoured")
	}
	if drawing.ForceRenderFrom(applyForceRender(base, forced, limiter, 0)) {
		t.Fatal("--force inside the cooldown honoured")
	}
	if forceRenderLimitKey(forced) == forceRenderLimitKey(plain) {
		t.Fatal("trigger fallback collided with command path key")
	}
	if forceRenderCooldown() < 0 {
		t.Fatal("default config disables force")
	}
}

func TestSekaiHandlerParsesForceFlag(t *testing.T) {
	handler := HarukiSekaiCommandHandler{}
	for _, tc := range []struct {
		args, rest string
		force      bool
	}{
		{"--force", "", true},
		{"ミク 强制刷新", "ミク", true},
		{"miku --FORCE -v", "miku", true},
		{"--forced", "--forced", false},
		{"强制刷新曲", "强制刷新曲", false},
	} {
		ctx := &PjskHandlerContext{Context: context.Background(), ArgText: tc.args}
		input := handler.parseHandlerInput(ctx, "/个人信息")
		if input.flags["is_force"] != tc.force || input.args != tc.rest {
			t.Fatalf("%q: force=%v args=%q", tc.args, input.flags["is_force"], input.args)
		}
	}
}
