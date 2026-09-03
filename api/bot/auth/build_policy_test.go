package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"haruki-cloud/config"
	"haruki-cloud/database/bot/user"
	"haruki-cloud/internal/core/buildpolicy"
	"haruki-cloud/internal/core/secevent"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	noiseMP "github.com/shamaton/msgpack/v3"
)

type recordingReporter struct {
	mu     sync.Mutex
	events []secevent.Event
}

func (r *recordingReporter) Report(_ context.Context, ev secevent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recordingReporter) find(kind secevent.Kind) *secevent.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.events {
		if r.events[i].Kind == kind {
			return &r.events[i]
		}
	}
	return nil
}

func policyStore(t *testing.T, mode buildpolicy.Mode, doc buildpolicy.Document) *buildpolicy.Store {
	t.Helper()
	raw, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return buildpolicy.NewStore(path, mode, nil)
}

func releasedPolicy() buildpolicy.Document {
	return buildpolicy.Document{
		Version: 1,
		Builds: []buildpolicy.Build{
			{BuildID: "build-abc123", Version: "2.9.0"},
			{BuildID: "build-def456", Version: "3.1.0", Target: "linux-amd64"},
		},
		RevokedVersions: []string{"2.8.*"},
	}
}

func sessionClaims(t *testing.T, body []byte) jwt.MapClaims {
	t.Helper()
	var resp AuthResponseV3
	if err := noiseMP.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	parsed, err := jwt.Parse(resp.SessionToken, func(*jwt.Token) (any, error) {
		return []byte(config.Cfg.HarukiBotDB.SessionSignToken), nil
	})
	if err != nil {
		t.Fatalf("parse session token: %v", err)
	}
	return parsed.Claims.(jwt.MapClaims)
}

func TestAuthV3BuildPolicyEnforceRejectsUnlistedBuild(t *testing.T) {
	env := newAuthV3TestEnv(t)
	reporter := &recordingReporter{}
	env.svc.WithBuildPolicy(policyStore(t, buildpolicy.ModeEnforce, releasedPolicy())).WithSecurityReporter(reporter)

	// Unknown build: refused with the generic message, event enforced.
	payload := env.basePayload(t)
	payload.BuildID = "build-forged"
	status, body := env.send(t, env.current, "", payload)
	if status != fiber.StatusForbidden || string(body) != ErrClientNotAuthorized {
		t.Fatalf("unlisted build: status %d body %q", status, body)
	}
	ev := reporter.find(secevent.KindBuildRejected)
	if ev == nil || !ev.Enforced || ev.BotID != env.botStr || ev.BuildID != "build-forged" || ev.Reason == "" {
		t.Fatalf("build_rejected event = %+v", ev)
	}

	// Version mismatch for a listed build is refused too.
	payload = env.basePayload(t)
	payload.ClientVersion = "2.9.1"
	if status, _ := env.send(t, env.current, "", payload); status != fiber.StatusForbidden {
		t.Fatalf("version mismatch: status %d", status)
	}
	// Target mismatch when the client reports one.
	payload = env.basePayload(t)
	payload.BuildID, payload.ClientVersion, payload.Target = "build-def456", "3.1.0", "windows-amd64"
	if status, _ := env.send(t, env.current, "", payload); status != fiber.StatusForbidden {
		t.Fatalf("target mismatch: status %d", status)
	}

	// The released pair logs in and the session pins the build identity.
	payload = env.basePayload(t)
	status, body = env.send(t, env.current, "", payload)
	if status != fiber.StatusOK {
		t.Fatalf("listed build: status %d body %q", status, body)
	}
	claims := sessionClaims(t, body)
	if claims["bid"] != "build-abc123" || claims["cv"] != "2.9.0" {
		t.Fatalf("session claims = %v", claims)
	}
	row, err := env.client.User.Query().Where(user.BotIDEQ(env.botID)).Only(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if row.LastBuildID != "build-abc123" || row.LastClientVersion != "2.9.0" {
		t.Fatalf("recorded client = %s/%s", row.LastClientVersion, row.LastBuildID)
	}
}

func TestAuthV3BuildPolicyRevokesBotsAndSources(t *testing.T) {
	env := newAuthV3TestEnv(t)
	reporter := &recordingReporter{}
	doc := releasedPolicy()
	doc.RevokedBots = []string{env.botStr}
	env.svc.WithBuildPolicy(policyStore(t, buildpolicy.ModeEnforce, doc)).WithSecurityReporter(reporter)

	if status, body := env.send(t, env.current, "", env.basePayload(t)); status != fiber.StatusForbidden || string(body) != ErrClientNotAuthorized {
		t.Fatalf("revoked bot: status %d body %q", status, body)
	}
	if ev := reporter.find(secevent.KindBuildRejected); ev == nil || ev.Reason != buildpolicy.CodeBotRevoked+": bot credential revoked" {
		t.Fatalf("event = %+v", ev)
	}

	// Revoked version prefix wins even for a build id that is unknown.
	doc = releasedPolicy()
	env.svc.WithBuildPolicy(policyStore(t, buildpolicy.ModeEnforce, doc))
	payload := env.basePayload(t)
	payload.ClientVersion = "2.8.7"
	if status, _ := env.send(t, env.current, "", payload); status != fiber.StatusForbidden {
		t.Fatalf("revoked version: status %d", status)
	}
}

func TestAuthV3BuildPolicyLogOnlyAdmitsAndReports(t *testing.T) {
	env := newAuthV3TestEnv(t)
	reporter := &recordingReporter{}
	env.svc.WithBuildPolicy(policyStore(t, buildpolicy.ModeLogOnly, releasedPolicy())).WithSecurityReporter(reporter)

	payload := env.basePayload(t)
	payload.BuildID = "build-unlisted"
	if status, body := env.send(t, env.current, "", payload); status != fiber.StatusOK {
		t.Fatalf("log-only must admit: status %d body %q", status, body)
	}
	ev := reporter.find(secevent.KindBuildRejected)
	if ev == nil || ev.Enforced || ev.BuildID != "build-unlisted" {
		t.Fatalf("event = %+v", ev)
	}

	// A missing policy file is reported as unavailable, never as a rejection.
	env.svc.WithBuildPolicy(buildpolicy.NewStore(filepath.Join(t.TempDir(), "missing.json"), buildpolicy.ModeEnforce, nil))
	if status, _ := env.send(t, env.current, "", env.basePayload(t)); status != fiber.StatusOK {
		t.Fatalf("missing policy must fail open: status %d", status)
	}
	if ev := reporter.find(secevent.KindPolicyUnavailable); ev == nil {
		t.Fatal("policy_unavailable event missing")
	}
}

func TestAuthV3ReportsFailuresReplaysAndClientChanges(t *testing.T) {
	env := newAuthV3TestEnv(t)
	reporter := &recordingReporter{}
	env.svc.WithSecurityReporter(reporter)

	bad := env.basePayload(t)
	bad.Credential = signTestCredential(t, env.botStr, "wrong-credential")
	if status, _ := env.send(t, env.current, "", bad); status != fiber.StatusBadRequest {
		t.Fatalf("bad credential: status %d", status)
	}
	if ev := reporter.find(secevent.KindAuthFailed); ev == nil || !ev.Enforced || ev.BotID != env.botStr {
		t.Fatalf("auth_failed event = %+v", ev)
	}

	first := env.basePayload(t)
	if status, _ := env.send(t, env.current, "", first); status != fiber.StatusOK {
		t.Fatal("first login failed")
	}
	env.expectReject(t, env.current, "", first, ErrReplayDetected)
	if ev := reporter.find(secevent.KindReplayDetected); ev == nil || ev.Reason != ErrReplayDetected {
		t.Fatalf("replay event = %+v", ev)
	}

	// Same bot, different build id: flagged, still admitted.
	changed := env.basePayload(t)
	changed.BuildID, changed.ClientVersion = "build-zzz", "2.9.5"
	if status, _ := env.send(t, env.current, "", changed); status != fiber.StatusOK {
		t.Fatal("changed client login failed")
	}
	ev := reporter.find(secevent.KindClientChanged)
	if ev == nil || ev.Enforced || ev.BuildID != "build-zzz" || ev.Reason != "previous client 2.9.0/build-abc123" {
		t.Fatalf("client_changed event = %+v", ev)
	}

	// Rate limit: the 11th login inside the window is refused and reported.
	for range RateLimitAuth {
		env.send(t, env.current, "", env.basePayload(t))
	}
	if ev := reporter.find(secevent.KindRateLimited); ev == nil || !ev.Enforced {
		t.Fatalf("rate_limited event = %+v", ev)
	}
}
