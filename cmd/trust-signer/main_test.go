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
