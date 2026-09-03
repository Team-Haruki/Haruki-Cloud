package buildpolicy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/core/trustsign"
)

func sampleDocument() Document {
	return Document{
		Version: 7,
		Builds: []Build{
			{BuildID: "b-310-linux", Version: "3.1.0", Target: "linux-amd64", SHA256: strings.Repeat("ab", 32)},
			{BuildID: "b-300-linux", Version: "3.0.0", Revoked: true, Reason: "leaked signing cert"},
			{BuildID: "b-320-rc", Version: "3.2.0", NotBefore: 2_000_000_000},
			{BuildID: "b-290-old", Version: "2.9.0", NotAfter: 1_000_000_000},
		},
		RevokedVersions: []string{"2.8.*", "3.0.1"},
		RevokedBots:     []string{"666"},
		BlockedSources:  []string{"203.0.113.7", "198.51.100.0/24"},
	}
}

func writeDoc(t *testing.T, dir string, doc Document) string {
	t.Helper()
	raw, _ := json.Marshal(doc)
	path := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func enforcingStore(t *testing.T, doc Document) *Store {
	t.Helper()
	store := NewStore(writeDoc(t, t.TempDir(), doc), ModeEnforce, nil)
	return store
}

func TestValidateRejectsBrokenDocuments(t *testing.T) {
	cases := map[string]func(d *Document){
		"zero version":    func(d *Document) { d.Version = 0 },
		"empty build id":  func(d *Document) { d.Builds[0].BuildID = " " },
		"empty version":   func(d *Document) { d.Builds[0].Version = "" },
		"duplicate build": func(d *Document) { d.Builds = append(d.Builds, Build{BuildID: "b-310-linux", Version: "x"}) },
		"bad sha256":      func(d *Document) { d.Builds[0].SHA256 = "zz" },
		"window inverted": func(d *Document) { d.Builds[0].NotBefore = 10; d.Builds[0].NotAfter = 5 },
		"bad source":      func(d *Document) { d.BlockedSources = []string{"not-an-ip"} },
		"expiry inverted": func(d *Document) { d.IssuedAt = 10; d.ExpiresAt = 5 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			doc := sampleDocument()
			mutate(&doc)
			if err := doc.Validate(); !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("Validate() = %v, want ErrInvalidDocument", err)
			}
		})
	}
	doc := sampleDocument()
	if err := doc.Validate(); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
}

func TestEvaluateMatrix(t *testing.T) {
	store := enforcingStore(t, sampleDocument())
	now := time.Unix(1_500_000_000, 0)
	base := Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux", SourceIP: "192.0.2.1", Now: now}

	cases := []struct {
		name   string
		mutate func(r *Request)
		code   string
	}{
		{"ok", func(*Request) {}, CodeOK},
		{"ok with matching target and hash", func(r *Request) { r.Target = "LINUX-AMD64"; r.BinarySHA256 = strings.ToUpper(strings.Repeat("ab", 32)) }, CodeOK},
		{"bot revoked", func(r *Request) { r.BotID = "666" }, CodeBotRevoked},
		{"source blocked exact", func(r *Request) { r.SourceIP = "203.0.113.7" }, CodeSourceBlocked},
		{"source blocked cidr", func(r *Request) { r.SourceIP = "198.51.100.200" }, CodeSourceBlocked},
		{"source blocked v4-mapped", func(r *Request) { r.SourceIP = "::ffff:203.0.113.7" }, CodeSourceBlocked},
		{"version revoked prefix", func(r *Request) { r.ClientVersion = "2.8.4"; r.BuildID = "whatever" }, CodeVersionRevoked},
		{"version revoked exact", func(r *Request) { r.ClientVersion = "3.0.1" }, CodeVersionRevoked},
		{"build missing", func(r *Request) { r.BuildID = "" }, CodeBuildMissing},
		{"build unknown", func(r *Request) { r.BuildID = "nope" }, CodeBuildUnknown},
		{"build revoked", func(r *Request) { r.BuildID = "b-300-linux"; r.ClientVersion = "3.0.0" }, CodeBuildRevoked},
		{"version mismatch", func(r *Request) { r.ClientVersion = "3.1.1" }, CodeBuildVersion},
		{"not yet valid", func(r *Request) { r.BuildID = "b-320-rc"; r.ClientVersion = "3.2.0" }, CodeBuildNotYetValid},
		{"expired", func(r *Request) { r.BuildID = "b-290-old"; r.ClientVersion = "2.9.0" }, CodeBuildExpired},
		{"target mismatch", func(r *Request) { r.Target = "windows-amd64" }, CodeBuildTarget},
		{"hash mismatch", func(r *Request) { r.BinarySHA256 = strings.Repeat("cd", 32) }, CodeBuildHash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.mutate(&req)
			d := store.Evaluate(req)
			if d.Code != tc.code {
				t.Fatalf("code = %s (%s), want %s", d.Code, d.Reason, tc.code)
			}
			if d.Allowed != (tc.code == CodeOK) || d.Passed != (tc.code == CodeOK) || !d.Enforce {
				t.Fatalf("decision = %+v", d)
			}
		})
	}
}

func TestModesAndNilStore(t *testing.T) {
	var nilStore *Store
	if d := nilStore.Evaluate(Request{BuildID: "x"}); !d.Allowed || d.Code != CodePolicyOff {
		t.Fatalf("nil store = %+v", d)
	}
	if d := nilStore.SessionAllowed("1", "1", "1", time.Now()); !d.Allowed {
		t.Fatalf("nil store session = %+v", d)
	}

	logOnly := NewStore(writeDoc(t, t.TempDir(), sampleDocument()), ModeLogOnly, nil)
	d := logOnly.Evaluate(Request{BotID: "1", ClientVersion: "9.9.9", BuildID: "nope"})
	if !d.Allowed || d.Passed || d.Code != CodeBuildUnknown || d.Enforce {
		t.Fatalf("log-only = %+v", d)
	}

	off := NewStore("/nonexistent", ModeOff, nil)
	if d := off.Evaluate(Request{}); !d.Allowed || d.Code != CodePolicyOff {
		t.Fatalf("off = %+v", d)
	}

	for raw, want := range map[string]Mode{"": ModeLogOnly, "off": ModeOff, "LOG-ONLY": ModeLogOnly, "enforce": ModeEnforce, "audit": ModeLogOnly} {
		got, err := ParseMode(raw, true)
		if err != nil || got != want {
			t.Fatalf("ParseMode(%q) = %s, %v; want %s", raw, got, err, want)
		}
	}
	if got, _ := ParseMode("", false); got != ModeOff {
		t.Fatalf("ParseMode(empty, no path) = %s", got)
	}
	if _, err := ParseMode("maybe", true); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestMissingOrExpiredPolicyFailsOpenWithCode(t *testing.T) {
	missing := NewStore(filepath.Join(t.TempDir(), "missing.json"), ModeEnforce, nil)
	d := missing.Evaluate(Request{BotID: "1", BuildID: "x", ClientVersion: "1"})
	if !d.Allowed || d.Passed || d.Code != CodePolicyUnavailable {
		t.Fatalf("missing file = %+v", d)
	}

	doc := sampleDocument()
	doc.IssuedAt = 1_000
	doc.ExpiresAt = 2_000
	expired := enforcingStore(t, doc)
	d = expired.Evaluate(Request{BotID: "1", BuildID: "b-310-linux", ClientVersion: "3.1.0", Now: time.Unix(3_000, 0)})
	if !d.Allowed || d.Code != CodePolicyUnavailable || !strings.Contains(d.Reason, "expired") {
		t.Fatalf("expired = %+v", d)
	}
}

func TestSessionAllowedOnlyAppliesRevocations(t *testing.T) {
	store := enforcingStore(t, sampleDocument())
	now := time.Unix(1_500_000_000, 0)
	if d := store.SessionAllowed("42", "3.1.0", "unknown-build", now); !d.Allowed || d.Code != CodeOK {
		t.Fatalf("unknown build must keep its session: %+v", d)
	}
	if d := store.SessionAllowed("666", "3.1.0", "b-310-linux", now); d.Allowed || d.Code != CodeBotRevoked {
		t.Fatalf("revoked bot = %+v", d)
	}
	if d := store.SessionAllowed("42", "2.8.9", "b-310-linux", now); d.Allowed || d.Code != CodeVersionRevoked {
		t.Fatalf("revoked version = %+v", d)
	}
	if d := store.SessionAllowed("42", "3.0.0", "b-300-linux", now); d.Allowed || d.Code != CodeBuildRevoked || d.Reason != "leaked signing cert" {
		t.Fatalf("revoked build = %+v", d)
	}
	if d := store.SessionAllowed("42", "2.9.0", "b-290-old", now); d.Allowed || d.Code != CodeBuildExpired {
		t.Fatalf("expired build = %+v", d)
	}
}

func TestStoreReloadsOnModTime(t *testing.T) {
	dir := t.TempDir()
	path := writeDoc(t, dir, sampleDocument())
	now := time.Unix(1_500_000_000, 0)
	store := NewStore(path, ModeEnforce, nil)
	store.now = func() time.Time { return now }
	req := Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux", Now: now}
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("initial = %+v", d)
	}

	doc := sampleDocument()
	doc.Builds[0].Revoked = true
	raw, _ := json.Marshal(doc)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("reloaded inside the interval: %+v", d)
	}
	now = now.Add(defaultReloadInterval + time.Second)
	req.Now = now
	if d := store.Evaluate(req); d.Code != CodeBuildRevoked {
		t.Fatalf("not reloaded after interval: %+v", d)
	}
}

func TestParseSignedEnvelope(t *testing.T) {
	root, err := trustsign.NewSigner("root", make([]byte, trustsign.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	other, _ := trustsign.NewSigner("other", append([]byte{1}, make([]byte, trustsign.SeedSize-1)...))
	doc := sampleDocument()
	payload, _ := json.Marshal(doc)
	env, err := root.Sign(trustsign.DomainRelease, trustsign.EncodingJSON, payload)
	if err != nil {
		t.Fatal(err)
	}
	signed, _ := json.Marshal(env)

	if got, err := Parse(signed, root.PublicKey()); err != nil || got.Version != 7 {
		t.Fatalf("verified parse = %+v, %v", got, err)
	}
	if got, err := Parse(signed, nil); err != nil || got.Version != 7 {
		t.Fatalf("unverified envelope parse = %+v, %v", got, err)
	}
	if _, err := Parse(signed, other.PublicKey()); err == nil {
		t.Fatal("wrong root accepted")
	}
	if _, err := Parse(payload, root.PublicKey()); err == nil {
		t.Fatal("bare document accepted despite a configured root")
	}
	wrongDomain, _ := root.Sign(trustsign.DomainKeyset, trustsign.EncodingJSON, payload)
	rawWrong, _ := json.Marshal(wrongDomain)
	if _, err := Parse(rawWrong, root.PublicKey()); err == nil {
		t.Fatal("keyset-domain envelope accepted as a release policy")
	}
	if _, err := Parse([]byte(`{"version":0}`), nil); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("invalid document error = %v", err)
	}
}
