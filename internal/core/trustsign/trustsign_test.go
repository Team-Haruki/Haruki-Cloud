package trustsign

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

func testSigner(t *testing.T, keyID string) *Signer {
	t.Helper()
	seed, err := GenerateSeed()
	if err != nil {
		t.Fatalf("GenerateSeed: %v", err)
	}
	signer, err := NewSigner(keyID, seed)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return signer
}

func TestSignerConstruction(t *testing.T) {
	if _, err := NewSigner("", make([]byte, SeedSize)); !errors.Is(err, ErrEmptyKeyID) {
		t.Fatalf("empty key id error = %v", err)
	}
	if _, err := NewSigner("k", make([]byte, 5)); !errors.Is(err, ErrBadSeed) {
		t.Fatalf("short seed error = %v", err)
	}
	if _, err := NewSignerFromHex("k", "zz"); err == nil {
		t.Fatal("bad hex accepted")
	}
	seed, _ := GenerateSeed()
	fromHex, err := NewSignerFromHex("k", hex.EncodeToString(seed))
	if err != nil {
		t.Fatalf("NewSignerFromHex: %v", err)
	}
	fromSeed, _ := NewSigner("k", seed)
	if fromHex.PublicKeyHex() != fromSeed.PublicKeyHex() || fromHex.KeyID() != "k" {
		t.Fatal("hex and raw seed produced different keys")
	}
	if _, err := ParsePublicKeyHex(fromHex.PublicKeyHex()); err != nil {
		t.Fatalf("ParsePublicKeyHex: %v", err)
	}
	if _, err := ParsePublicKeyHex("abcd"); !errors.Is(err, ErrBadPublicKey) {
		t.Fatalf("short public key error = %v", err)
	}
}

func TestSignVerifyRoundTripAndJSONShape(t *testing.T) {
	signer := testSigner(t, "manifest-2026-09")
	payload := []byte(`{"entries":[],"profile":"production"}`)
	env, err := signer.Sign(DomainManifest, EncodingJSON, payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if env.Algorithm != Algorithm || env.Domain != DomainManifest || env.KeyID != "manifest-2026-09" || env.Encoding != EncodingJSON {
		t.Fatalf("envelope metadata = %+v", env)
	}
	if err := Verify(signer.PublicKey(), env, DomainManifest); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := Verify(signer.PublicKey(), env, ""); err != nil {
		t.Fatalf("Verify without domain pin: %v", err)
	}

	// Mutating the caller's payload after signing must not affect the envelope.
	payload[2] = 'X'
	if err := Verify(signer.PublicKey(), env, DomainManifest); err != nil {
		t.Fatalf("envelope shares memory with caller payload: %v", err)
	}

	// Wire shape: payload and signature are base64 strings in JSON.
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	for _, field := range []string{"alg", "domain", "key_id", "encoding", "payload", "signature"} {
		if _, ok := wire[field].(string); !ok {
			t.Fatalf("wire field %q = %#v, want string", field, wire[field])
		}
	}
	var decoded Envelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if err := Verify(signer.PublicKey(), decoded, DomainManifest); err != nil {
		t.Fatalf("Verify after JSON round trip: %v", err)
	}
}

func TestVerifyRejectsTamperingAndDomainReplay(t *testing.T) {
	signer := testSigner(t, "root")
	other := testSigner(t, "other")
	env, _ := signer.Sign(DomainKeyset, EncodingJSON, []byte(`{"version":1}`))

	if err := Verify(other.PublicKey(), env, DomainKeyset); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("wrong key error = %v", err)
	}
	if err := Verify(signer.PublicKey(), env, DomainManifest); !errors.Is(err, ErrDomainMismatch) {
		t.Fatalf("domain pin error = %v", err)
	}

	tampered := env
	tampered.Payload = []byte(`{"version":2}`)
	if err := Verify(signer.PublicKey(), tampered, DomainKeyset); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered payload error = %v", err)
	}

	// Relabelling the domain without re-signing must fail even without a pin:
	// the domain is part of the signed input.
	relabelled := env
	relabelled.Domain = DomainManifest
	if err := Verify(signer.PublicKey(), relabelled, ""); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("relabelled domain error = %v", err)
	}

	badAlg := env
	badAlg.Algorithm = "rsa"
	if err := Verify(signer.PublicKey(), badAlg, DomainKeyset); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Fatalf("algorithm error = %v", err)
	}
	if err := Verify([]byte{1, 2, 3}, env, DomainKeyset); !errors.Is(err, ErrBadPublicKey) {
		t.Fatalf("short key error = %v", err)
	}
	if _, err := signer.Sign("bad\x00domain", EncodingJSON, nil); !errors.Is(err, ErrBadDomain) {
		t.Fatalf("NUL domain error = %v", err)
	}
	if _, err := SigningInput("", nil); !errors.Is(err, ErrBadDomain) {
		t.Fatalf("empty domain error = %v", err)
	}
}

func TestKeysetDocumentValidate(t *testing.T) {
	pub := testSigner(t, "x").PublicKeyHex()
	valid := KeysetDocument{
		Version:             3,
		IssuedAt:            1_700_000_000,
		ExpiresAt:           1_700_600_000,
		NoiseKeys:           []KeysetKey{{KeyID: "noise-a", PublicKey: pub}, {KeyID: "noise-b", PublicKey: pub, NotBefore: 1, NotAfter: 2}},
		ManifestSigningKeys: []KeysetKey{{KeyID: "manifest-a", PublicKey: pub}},
		Endpoints:           map[string]string{"production": "https://api.example.com"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}

	cases := map[string]func(*KeysetDocument){
		"zero version":        func(d *KeysetDocument) { d.Version = 0 },
		"expires before":      func(d *KeysetDocument) { d.ExpiresAt = d.IssuedAt },
		"no noise keys":       func(d *KeysetDocument) { d.NoiseKeys = nil },
		"empty key id":        func(d *KeysetDocument) { d.NoiseKeys[0].KeyID = " " },
		"duplicate key id":    func(d *KeysetDocument) { d.NoiseKeys[1].KeyID = "noise-a" },
		"bad public key":      func(d *KeysetDocument) { d.ManifestSigningKeys[0].PublicKey = "abcd" },
		"inverted validity":   func(d *KeysetDocument) { d.NoiseKeys[1].NotAfter = 1 },
		"plain http endpoint": func(d *KeysetDocument) { d.Endpoints["alt"] = "http://x" },
		"blank endpoint name": func(d *KeysetDocument) { d.Endpoints[" "] = "https://x" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			doc := valid
			doc.NoiseKeys = append([]KeysetKey(nil), valid.NoiseKeys...)
			doc.ManifestSigningKeys = append([]KeysetKey(nil), valid.ManifestSigningKeys...)
			doc.Endpoints = map[string]string{"production": "https://api.example.com"}
			mutate(&doc)
			if err := doc.Validate(); err == nil {
				t.Fatal("invalid document accepted")
			}
		})
	}
}
