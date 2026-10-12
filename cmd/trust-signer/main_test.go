package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haruki-cloud/internal/core/trustsign"
)

func runOK(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run(%v) = %d, stderr: %s", args, code, stderr.String())
	}
	return stdout.String()
}

func runFail(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code == 0 {
		t.Fatalf("run(%v) succeeded, stdout: %s", args, stdout.String())
	}
	return stderr.String()
}

func TestKeygenSignVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	seedFile := filepath.Join(dir, "root.seed")

	var keygen struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal([]byte(runOK(t, "keygen", "--out", seedFile, "--key-id", "root-2026")), &keygen); err != nil {
		t.Fatalf("decode keygen output: %v", err)
	}
	if keygen.KeyID != "root-2026" || len(keygen.PublicKey) != 64 {
		t.Fatalf("keygen output = %+v", keygen)
	}
	info, err := os.Stat(seedFile)
	if err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("seed file mode = %v, want 0600", info.Mode().Perm())
	}
	if msg := runFail(t, "keygen", "--out", seedFile, "--key-id", "root-2026"); !strings.Contains(msg, "refusing to overwrite") {
		t.Fatalf("overwrite protection message = %q", msg)
	}

	doc := trustsign.KeysetDocument{
		Version:   1,
		IssuedAt:  1_700_000_000,
		ExpiresAt: 1_700_600_000,
		NoiseKeys: []trustsign.KeysetKey{{KeyID: "noise-2026-09", PublicKey: keygen.PublicKey}},
		Endpoints: map[string]string{"production": "https://api.example.com"},
	}
	payload, _ := json.Marshal(doc)
	payloadFile := filepath.Join(dir, "keyset.json")
	if err := os.WriteFile(payloadFile, payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	envelopeFile := filepath.Join(dir, "keyset.signed.json")
	runOK(t, "sign", "--key", seedFile, "--key-id", "root-2026", "--domain", "keyset", "--in", payloadFile, "--out", envelopeFile)

	verified := runOK(t, "verify", "--public", keygen.PublicKey, "--in", envelopeFile, "--domain", "keyset")
	if verified != string(payload) {
		t.Fatalf("verify printed %q, want the exact payload bytes", verified)
	}

	// The envelope on disk is the wire contract the Cloud serves verbatim.
	raw, _ := os.ReadFile(envelopeFile)
	var envelope trustsign.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Domain != trustsign.DomainKeyset || envelope.KeyID != "root-2026" || envelope.Encoding != trustsign.EncodingJSON {
		t.Fatalf("envelope metadata = %+v", envelope)
	}

	// Wrong domain pin and wrong key both fail verification.
	if msg := runFail(t, "verify", "--public", keygen.PublicKey, "--in", envelopeFile, "--domain", "manifest"); !strings.Contains(msg, "domain") {
		t.Fatalf("domain mismatch message = %q", msg)
	}
	other := runOK(t, "keygen", "--out", filepath.Join(dir, "other.seed"), "--key-id", "other")
	var otherKey struct {
		PublicKey string `json:"public_key"`
	}
	_ = json.Unmarshal([]byte(other), &otherKey)
	if msg := runFail(t, "verify", "--public", otherKey.PublicKey, "--in", envelopeFile); !strings.Contains(msg, "verification failed") {
		t.Fatalf("wrong key message = %q", msg)
	}
}

func TestSignRejectsInvalidKeysetAndBadFlags(t *testing.T) {
	dir := t.TempDir()
	seedFile := filepath.Join(dir, "root.seed")
	runOK(t, "keygen", "--out", seedFile, "--key-id", "root")

	badPayload := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPayload, []byte(`{"version":0}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := filepath.Join(dir, "out.json")
	if msg := runFail(t, "sign", "--key", seedFile, "--key-id", "root", "--domain", "keyset", "--in", badPayload, "--out", out); !strings.Contains(msg, "keyset payload rejected") {
		t.Fatalf("invalid keyset message = %q", msg)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("envelope was written for an invalid keyset")
	}

	// Manifest domain skips keyset validation: arbitrary bytes are fine.
	runOK(t, "sign", "--key", seedFile, "--key-id", "root", "--domain", "manifest", "--in", badPayload, "--out", out)

	runFail(t, "sign", "--key", seedFile, "--key-id", "root", "--domain", "nope", "--in", badPayload, "--out", out)
	runFail(t, "sign", "--key", seedFile, "--key-id", "root", "--domain", "keyset", "--in", badPayload, "--out", out, "--encoding", "msgpack")
	runFail(t, "sign", "--key", seedFile)
	runFail(t, "keygen")
	runFail(t, "verify", "--public", "zz", "--in", out)
	runFail(t, "bogus")
	runFail(t)
	if help := runOK(t, "help"); !strings.Contains(help, "keygen") {
		t.Fatalf("help output = %q", help)
	}
}

func TestSignReleasePolicyValidatesAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	seedFile := filepath.Join(dir, "root.seed")
	var keygen struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal([]byte(runOK(t, "keygen", "--out", seedFile, "--key-id", "root")), &keygen); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(dir, "bad-policy.json")
	if err := os.WriteFile(bad, []byte(`{"version":1,"builds":[{"build_id":"","version":"1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "policy.signed.json")
	if msg := runFail(t, "sign", "--key", seedFile, "--key-id", "root", "--domain", "release", "--in", bad, "--out", out); !strings.Contains(msg, "release policy payload rejected") {
		t.Fatalf("invalid policy message = %q", msg)
	}

	good := filepath.Join(dir, "policy.json")
	payload := []byte(`{"version":3,"builds":[{"build_id":"b1","version":"3.1.0"}],"revoked_versions":["3.0.*"]}`)
	if err := os.WriteFile(good, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	runOK(t, "sign", "--key", seedFile, "--key-id", "root", "--domain", "release", "--in", good, "--out", out)
	if verified := runOK(t, "verify", "--public", keygen.PublicKey, "--in", out, "--domain", "release"); verified != string(payload) {
		t.Fatalf("verify printed %q", verified)
	}
	raw, _ := os.ReadFile(out)
	var envelope trustsign.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Domain != trustsign.DomainRelease {
		t.Fatalf("domain = %q", envelope.Domain)
	}
}

func TestKeysetAddReleaseKeyAndDelegatedVerify(t *testing.T) {
	dir := t.TempDir()
	rootSeed := filepath.Join(dir, "root.seed")
	releaseSeed := filepath.Join(dir, "release.seed")
	var root, release struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal([]byte(runOK(t, "keygen", "--out", rootSeed, "--key-id", "root")), &root); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(runOK(t, "keygen", "--out", releaseSeed, "--key-id", "release-test")), &release); err != nil {
		t.Fatal(err)
	}

	keysetV3 := filepath.Join(dir, "keyset-v3.json")
	v3 := trustsign.KeysetDocument{
		Version:   3,
		IssuedAt:  1_700_000_000,
		ExpiresAt: 4_000_000_000,
		NoiseKeys: []trustsign.KeysetKey{{KeyID: "noise", PublicKey: root.PublicKey}},
		Endpoints: map[string]string{"production": "https://api.example.com"},
	}
	rawV3, _ := json.Marshal(v3)
	if err := os.WriteFile(keysetV3, rawV3, 0o600); err != nil {
		t.Fatal(err)
	}

	keysetV4 := filepath.Join(dir, "keyset-v4.json")
	var added struct {
		KeyID       string `json:"key_id"`
		PublicKey   string `json:"public_key"`
		NotBefore   int64  `json:"not_before"`
		NotAfter    int64  `json:"not_after"`
		Version     int64  `json:"version"`
		ReleaseKeys int    `json:"release_keys"`
	}
	out := runOK(t, "keyset-add-release-key", "--in", keysetV3, "--out", keysetV4, "--seed", releaseSeed,
		"--key-id", "release-test", "--not-before", "2026-01-01T00:00:00Z", "--not-after", "3900000000")
	if err := json.Unmarshal([]byte(out), &added); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if added.KeyID != "release-test" || added.PublicKey != release.PublicKey || added.Version != 4 || added.ReleaseKeys != 1 ||
		added.NotBefore != 1_767_225_600 || added.NotAfter != 3_900_000_000 {
		t.Fatalf("output = %+v", added)
	}
	rawV4, _ := os.ReadFile(keysetV4)
	var v4 trustsign.KeysetDocument
	if err := json.Unmarshal(rawV4, &v4); err != nil {
		t.Fatal(err)
	}
	if v4.Version != 4 || v4.IssuedAt <= v3.IssuedAt || v4.ExpiresAt != v3.ExpiresAt || len(v4.NoiseKeys) != 1 ||
		v4.Endpoints["production"] != "https://api.example.com" || len(v4.ReleaseSigningKeys) != 1 ||
		v4.ReleaseSigningKeys[0].PublicKey != release.PublicKey {
		t.Fatalf("keyset v4 = %+v", v4)
	}
	if msg := runFail(t, "keyset-add-release-key", "--in", keysetV3, "--out", keysetV4, "--seed", releaseSeed, "--key-id", "x", "--not-after", "1"); !strings.Contains(msg, "refusing to overwrite") {
		t.Fatalf("overwrite message = %q", msg)
	}
	runFail(t, "keyset-add-release-key", "--in", keysetV3, "--out", filepath.Join(dir, "v4b.json"), "--seed", releaseSeed, "--key-id", "x")
	runFail(t, "keyset-add-release-key", "--in", keysetV3, "--out", filepath.Join(dir, "v4c.json"), "--seed", releaseSeed, "--key-id", "x", "--not-after", "soon")
	if msg := runFail(t, "keyset-add-release-key", "--in", keysetV3, "--out", filepath.Join(dir, "v4d.json"), "--seed", releaseSeed, "--key-id", "x", "--not-after", "1", "--version", "3"); !strings.Contains(msg, "must exceed") {
		t.Fatalf("version message = %q", msg)
	}

	// Root signs the keyset; the release key signs the policy; verify checks
	// the chain the way the Cloud does.
	signedKeyset := filepath.Join(dir, "keyset-v4.signed.json")
	runOK(t, "sign", "--key", rootSeed, "--key-id", "root", "--domain", "keyset", "--in", keysetV4, "--out", signedKeyset)
	policy := filepath.Join(dir, "policy.json")
	payload := []byte(`{"version":9,"builds":[{"build_id":"b1","version":"3.2.0"}]}`)
	if err := os.WriteFile(policy, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	signedPolicy := filepath.Join(dir, "policy.signed.json")
	runOK(t, "sign", "--key", releaseSeed, "--key-id", "release-test", "--domain", "release", "--in", policy, "--out", signedPolicy)

	if got := runOK(t, "verify", "--public", root.PublicKey, "--keyset", signedKeyset, "--in", signedPolicy, "--domain", "release"); got != string(payload) {
		t.Fatalf("delegated verify printed %q", got)
	}
	// Without the keyset the release key is unknown to the root.
	if msg := runFail(t, "verify", "--public", root.PublicKey, "--in", signedPolicy, "--domain", "release"); !strings.Contains(msg, "verification failed") {
		t.Fatalf("root-only verify message = %q", msg)
	}
	// Root-signed policies still verify through the keyset path.
	rootPolicy := filepath.Join(dir, "policy.root.signed.json")
	runOK(t, "sign", "--key", rootSeed, "--key-id", "root", "--domain", "release", "--in", policy, "--out", rootPolicy)
	if got := runOK(t, "verify", "--public", root.PublicKey, "--keyset", signedKeyset, "--in", rootPolicy, "--domain", "release"); got != string(payload) {
		t.Fatalf("root verify through keyset printed %q", got)
	}
	// A keyset signed by someone else is rejected before the policy is looked at.
	otherKeyset := filepath.Join(dir, "keyset.other.signed.json")
	runOK(t, "sign", "--key", releaseSeed, "--key-id", "release-test", "--domain", "keyset", "--in", keysetV4, "--out", otherKeyset)
	if msg := runFail(t, "verify", "--public", root.PublicKey, "--keyset", otherKeyset, "--in", signedPolicy, "--domain", "release"); !strings.Contains(msg, "keyset rejected") {
		t.Fatalf("foreign keyset message = %q", msg)
	}
	if msg := runFail(t, "verify", "--public", root.PublicKey, "--keyset", signedKeyset, "--in", signedKeyset, "--domain", "keyset"); !strings.Contains(msg, "release only") {
		t.Fatalf("keyset flag on keyset domain message = %q", msg)
	}
}
