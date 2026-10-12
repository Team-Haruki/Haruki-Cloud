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

// Fixed instants for the delegation tests: the release key is valid between
// keyValidFrom and keyValidTo, the keyset until keysetExpiry.
const (
	keyValidFrom = int64(1_600_000_000)
	keyValidTo   = int64(1_700_000_000)
	keysetExpiry = int64(1_800_000_000)
)

var inWindow = time.Unix(1_650_000_000, 0)

func throwawaySigner(t *testing.T, keyID string) *trustsign.Signer {
	t.Helper()
	seed, err := trustsign.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := trustsign.NewSigner(keyID, seed)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func keysetWith(release ...trustsign.KeysetKey) trustsign.KeysetDocument {
	return trustsign.KeysetDocument{
		Version:            4,
		IssuedAt:           keyValidFrom,
		ExpiresAt:          keysetExpiry,
		NoiseKeys:          []trustsign.KeysetKey{{KeyID: "noise", PublicKey: strings.Repeat("00", 32)}},
		ReleaseSigningKeys: release,
	}
}

func releaseEntry(signer *trustsign.Signer) trustsign.KeysetKey {
	return trustsign.KeysetKey{KeyID: signer.KeyID(), PublicKey: signer.PublicKeyHex(), NotBefore: keyValidFrom, NotAfter: keyValidTo}
}

func signKeyset(t *testing.T, root *trustsign.Signer, doc trustsign.KeysetDocument) []byte {
	t.Helper()
	payload, _ := json.Marshal(doc)
	env, err := root.Sign(trustsign.DomainKeyset, trustsign.EncodingJSON, payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(env)
	return raw
}

func signPolicy(t *testing.T, signer *trustsign.Signer, domain string) []byte {
	t.Helper()
	payload, _ := json.Marshal(sampleDocument())
	env, err := signer.Sign(domain, trustsign.EncodingJSON, payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(env)
	return raw
}

func TestVerifierDelegatedReleaseKey(t *testing.T) {
	root := throwawaySigner(t, "root-test")
	release := throwawaySigner(t, "release-test")
	stranger := throwawaySigner(t, "stranger")
	keyset := keysetWith(releaseEntry(release))
	verifier := func(doc trustsign.KeysetDocument, now time.Time) *Verifier {
		return &Verifier{Root: root.PublicKey(), Keyset: &doc, Now: now}
	}

	rootSigned := signPolicy(t, root, trustsign.DomainRelease)
	delegated := signPolicy(t, release, trustsign.DomainRelease)

	t.Run("root-direct still verifies with a keyset loaded", func(t *testing.T) {
		doc, err := Parse(rootSigned, verifier(keyset, inWindow))
		if err != nil || doc.SignedBy != "root-test" || doc.VerifiedUntil != 0 {
			t.Fatalf("Parse = %+v, %v", doc, err)
		}
	})
	t.Run("root-direct verifies without a keyset", func(t *testing.T) {
		if _, err := Parse(rootSigned, &Verifier{Root: root.PublicKey(), Now: inWindow}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("delegated verifies inside the window", func(t *testing.T) {
		doc, err := Parse(delegated, verifier(keyset, inWindow))
		if err != nil || doc.SignedBy != "release-test" || doc.VerifiedUntil != keyValidTo {
			t.Fatalf("Parse = %+v, %v", doc, err)
		}
		tight := keysetWith(releaseEntry(release))
		tight.ExpiresAt = keyValidTo - 1000
		if doc, err := Parse(delegated, verifier(tight, inWindow)); err != nil || doc.VerifiedUntil != tight.ExpiresAt-1 {
			t.Fatalf("keyset-bounded Parse = %+v, %v", doc, err)
		}
	})
	t.Run("delegated rejected without a keyset", func(t *testing.T) {
		if _, err := Parse(delegated, &Verifier{Root: root.PublicKey(), Now: inWindow}); !errors.Is(err, trustsign.ErrBadSignature) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("expired release key", func(t *testing.T) {
		_, err := Parse(delegated, verifier(keyset, time.Unix(keyValidTo+1, 0)))
		if err == nil || !strings.Contains(err.Error(), "validity window") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("release key not yet valid", func(t *testing.T) {
		_, err := Parse(delegated, verifier(keyset, time.Unix(keyValidFrom-1, 0)))
		if err == nil || !strings.Contains(err.Error(), "validity window") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("expired keyset disables delegation but not the root", func(t *testing.T) {
		late := time.Unix(keysetExpiry, 0)
		if _, err := Parse(delegated, verifier(keyset, late)); err == nil || !strings.Contains(err.Error(), "keyset has expired") {
			t.Fatalf("err = %v", err)
		}
		if _, err := Parse(rootSigned, verifier(keyset, late)); err != nil {
			t.Fatalf("root-direct after keyset expiry: %v", err)
		}
	})
	t.Run("unknown key id falls through to the root and fails", func(t *testing.T) {
		unknown := signPolicy(t, stranger, trustsign.DomainRelease)
		if _, err := Parse(unknown, verifier(keyset, inWindow)); !errors.Is(err, trustsign.ErrBadSignature) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("listed key id with another key's signature", func(t *testing.T) {
		impostor := throwawaySigner(t, "release-test")
		if _, err := Parse(signPolicy(t, impostor, trustsign.DomainRelease), verifier(keyset, inWindow)); !errors.Is(err, trustsign.ErrBadSignature) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("key dropped from the keyset is revoked", func(t *testing.T) {
		if _, err := Parse(delegated, verifier(keysetWith(), inWindow)); !errors.Is(err, trustsign.ErrBadSignature) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("wrong domain", func(t *testing.T) {
		if _, err := Parse(signPolicy(t, release, trustsign.DomainKeyset), verifier(keyset, inWindow)); err == nil || !strings.Contains(err.Error(), "domain") {
			t.Fatalf("err = %v", err)
		}
		manifest := signPolicy(t, release, trustsign.DomainManifest)
		if _, err := Parse(manifest, verifier(keyset, inWindow)); err == nil {
			t.Fatal("manifest-domain envelope accepted as a release policy")
		}
	})
	t.Run("tampered payload", func(t *testing.T) {
		var env trustsign.Envelope
		if err := json.Unmarshal(delegated, &env); err != nil {
			t.Fatal(err)
		}
		doc := sampleDocument()
		doc.RevokedBots = nil
		env.Payload, _ = json.Marshal(doc)
		tampered, _ := json.Marshal(env)
		if _, err := Parse(tampered, verifier(keyset, inWindow)); !errors.Is(err, trustsign.ErrBadSignature) {
			t.Fatalf("err = %v", err)
		}
	})
}

// writeFile writes raw and bumps the mtime past any previous write so the
// store's mtime check sees the change.
func writeFile(t *testing.T, path string, raw []byte, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHonoursKeysetAndReloadsOnKeysetChange(t *testing.T) {
	root := throwawaySigner(t, "root-test")
	release := throwawaySigner(t, "release-test")
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.signed.json")
	keysetPath := filepath.Join(dir, "keyset.signed.json")
	base := time.Now().Add(-time.Hour)
	writeFile(t, policyPath, signPolicy(t, release, trustsign.DomainRelease), base)
	writeFile(t, keysetPath, signKeyset(t, root, keysetWith(releaseEntry(release))), base)

	now := inWindow
	store := NewStore(policyPath, ModeEnforce, root.PublicKey(), keysetPath)
	store.now = func() time.Time { return now }
	req := Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux", Now: now}
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("delegated policy = %+v", d)
	}
	doc, _ := store.Document()
	if doc.SignedBy != "release-test" {
		t.Fatalf("SignedBy = %q", doc.SignedBy)
	}

	// Keyset v5 drops the release key: the untouched policy file must stop
	// verifying once the keyset's mtime is noticed.
	writeFile(t, keysetPath, signKeyset(t, root, keysetWith()), base.Add(time.Minute))
	now = now.Add(defaultReloadInterval + time.Second)
	req.Now = now
	d := store.Evaluate(req)
	if d.Code != CodePolicyUnavailable || !strings.Contains(d.Reason, "signature rejected") {
		t.Fatalf("after revocation = %+v", d)
	}

	// Root-signed policy keeps working against the key-less keyset.
	writeFile(t, policyPath, signPolicy(t, root, trustsign.DomainRelease), base.Add(2*time.Minute))
	now = now.Add(defaultReloadInterval + time.Second)
	req.Now = now
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("root-signed after revocation = %+v", d)
	}
}

func TestStoreRejectsBrokenKeyset(t *testing.T) {
	root := throwawaySigner(t, "root-test")
	other := throwawaySigner(t, "other-root")
	release := throwawaySigner(t, "release-test")
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.signed.json")
	keysetPath := filepath.Join(dir, "keyset.signed.json")
	mtime := time.Now().Add(-time.Hour)
	writeFile(t, policyPath, signPolicy(t, root, trustsign.DomainRelease), mtime)

	cases := map[string][]byte{
		"signed by another root": signKeyset(t, other, keysetWith(releaseEntry(release))),
		"not an envelope":        []byte(`{"version":4}`),
		"release key without not_after": signKeyset(t, root, keysetWith(trustsign.KeysetKey{
			KeyID: "release-test", PublicKey: release.PublicKeyHex(),
		})),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			writeFile(t, keysetPath, raw, mtime)
			store := NewStore(policyPath, ModeEnforce, root.PublicKey(), keysetPath)
			store.now = func() time.Time { return inWindow }
			d := store.Evaluate(Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux", Now: inWindow})
			if d.Code != CodePolicyUnavailable || !strings.Contains(d.Reason, "keyset") {
				t.Fatalf("decision = %+v", d)
			}
		})
	}

	t.Run("missing keyset file", func(t *testing.T) {
		store := NewStore(policyPath, ModeEnforce, root.PublicKey(), filepath.Join(dir, "missing.json"))
		if d := store.Evaluate(Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux", Now: inWindow}); d.Code != CodePolicyUnavailable {
			t.Fatalf("decision = %+v", d)
		}
	})
	t.Run("keyset path ignored without a root key", func(t *testing.T) {
		store := NewStore(policyPath, ModeEnforce, nil, filepath.Join(dir, "missing.json"))
		if d := store.Evaluate(Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux", Now: inWindow}); d.Code != CodeOK {
			t.Fatalf("decision = %+v", d)
		}
	})
}

func TestStoreDropsCachedPolicyWhenReleaseKeyLapses(t *testing.T) {
	root := throwawaySigner(t, "root-test")
	release := throwawaySigner(t, "release-test")
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.signed.json")
	keysetPath := filepath.Join(dir, "keyset.signed.json")
	mtime := time.Now().Add(-time.Hour)
	writeFile(t, policyPath, signPolicy(t, release, trustsign.DomainRelease), mtime)
	writeFile(t, keysetPath, signKeyset(t, root, keysetWith(releaseEntry(release))), mtime)

	now := time.Unix(keyValidTo-1, 0)
	store := NewStore(policyPath, ModeEnforce, root.PublicKey(), keysetPath)
	store.now = func() time.Time { return now }
	req := Request{BotID: "42", ClientVersion: "3.1.0", BuildID: "b-310-linux"}
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("before expiry = %+v", d)
	}

	// Neither file changes; only the clock passes not_after.
	now = time.Unix(keyValidTo+1, 0)
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("inside the reload interval the cache still serves: %+v", d)
	}
	now = now.Add(defaultReloadInterval)
	d := store.Evaluate(req)
	if d.Code != CodePolicyUnavailable || !strings.Contains(d.Reason, "validity window") {
		t.Fatalf("after expiry = %+v", d)
	}

	// Same with the keyset's own expiry bounding an otherwise valid key.
	tight := keysetWith(releaseEntry(release))
	tight.ExpiresAt = keyValidTo - 10_000
	writeFile(t, keysetPath, signKeyset(t, root, tight), mtime.Add(time.Minute))
	now = time.Unix(tight.ExpiresAt-1, 0)
	store = NewStore(policyPath, ModeEnforce, root.PublicKey(), keysetPath)
	store.now = func() time.Time { return now }
	if d := store.Evaluate(req); d.Code != CodeOK {
		t.Fatalf("before keyset expiry = %+v", d)
	}
	now = time.Unix(tight.ExpiresAt, 0).Add(defaultReloadInterval)
	if d := store.Evaluate(req); d.Code != CodePolicyUnavailable || !strings.Contains(d.Reason, "keyset has expired") {
		t.Fatalf("after keyset expiry = %+v", d)
	}
}
