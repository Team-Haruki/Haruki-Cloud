package crypto

import (
	"errors"
	"testing"
)

func testStaticKey(t *testing.T, id string) StaticKey {
	t.Helper()
	pair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return StaticKey{ID: id, Pair: pair}
}

func nkMessage1(t *testing.T, serverPub []byte, payload string) []byte {
	t.Helper()
	initiator, err := NewInitiator(serverPub)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	msg, err := initiator.EncryptPacket([]byte(payload))
	if err != nil {
		t.Fatalf("EncryptPacket: %v", err)
	}
	return msg
}

func TestNewKeyRingValidation(t *testing.T) {
	primary := testStaticKey(t, "k1")
	if _, err := NewKeyRing(); !errors.Is(err, errKeyRingEmpty) {
		t.Fatalf("empty ring error = %v", err)
	}
	if _, err := NewKeyRing(StaticKey{ID: "k1"}); !errors.Is(err, errStaticKeyMissing) {
		t.Fatalf("missing pair error = %v", err)
	}
	if _, err := NewKeyRing(StaticKey{Pair: primary.Pair}); !errors.Is(err, errStaticKeyNoID) {
		t.Fatalf("missing id error = %v", err)
	}
	if _, err := NewKeyRing(StaticKey{ID: "bad id", Pair: primary.Pair}); !errors.Is(err, errStaticKeyBadID) {
		t.Fatalf("whitespace id error = %v", err)
	}
	if _, err := NewKeyRing(primary, StaticKey{ID: "k1", Pair: testStaticKey(t, "x").Pair}); !errors.Is(err, errStaticKeyDupID) {
		t.Fatalf("duplicate id error = %v", err)
	}
	if _, err := NewKeyRing(primary, StaticKey{ID: "k2", Pair: primary.Pair}); !errors.Is(err, errStaticKeyDupPriv) {
		t.Fatalf("duplicate private key error = %v", err)
	}
	if _, err := NewKeyRing(StaticKey{ID: "short", Pair: &KeyPair{Private: []byte{1}, Public: primary.Pair.Public}}); !errors.Is(err, errStaticKeyBadPriv) {
		t.Fatalf("short private key error = %v", err)
	}

	ring, err := NewKeyRing(primary, testStaticKey(t, "k2"))
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}
	if ring.Len() != 2 || ring.Primary().ID != "k1" {
		t.Fatalf("ring = len %d primary %q", ring.Len(), ring.Primary().ID)
	}
	if _, ok := ring.Lookup("k2"); !ok {
		t.Fatal("Lookup(k2) = false")
	}
	if _, ok := ring.Lookup("missing"); ok {
		t.Fatal("Lookup(missing) = true")
	}
	keys := ring.Keys()
	keys[0] = StaticKey{}
	if ring.Primary().ID != "k1" {
		t.Fatal("Keys() must return a copy")
	}
}

func TestSingleKeyRingUsesDefaultID(t *testing.T) {
	pair := testStaticKey(t, "ignored").Pair
	ring, err := SingleKeyRing(pair)
	if err != nil {
		t.Fatalf("SingleKeyRing: %v", err)
	}
	if ring.Len() != 1 || ring.Primary().ID != DefaultKeyID {
		t.Fatalf("single ring = len %d primary %q", ring.Len(), ring.Primary().ID)
	}
	if _, err := SingleKeyRing(nil); err == nil {
		t.Fatal("SingleKeyRing(nil) succeeded")
	}
}

func TestKeyRingOpenNKMatchesAnyConfiguredKey(t *testing.T) {
	current := testStaticKey(t, "current")
	next := testStaticKey(t, "next")
	ring, err := NewKeyRing(current, next)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}

	for _, tc := range []struct {
		name      string
		target    StaticKey
		preferred string
	}{
		{name: "primary without hint", target: current},
		{name: "secondary without hint", target: next},
		{name: "secondary with matching hint", target: next, preferred: "next"},
		{name: "secondary with wrong hint", target: next, preferred: "current"},
		{name: "primary with unknown hint", target: current, preferred: "does-not-exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := nkMessage1(t, tc.target.Pair.Public, "hello")
			responder, plaintext, keyID, err := ring.OpenNK(msg, tc.preferred)
			if err != nil {
				t.Fatalf("OpenNK: %v", err)
			}
			if string(plaintext) != "hello" || keyID != tc.target.ID {
				t.Fatalf("OpenNK = payload %q key %q", plaintext, keyID)
			}
			if _, err := responder.EncryptPacket([]byte("reply")); err != nil {
				t.Fatalf("responder cannot continue handshake: %v", err)
			}
		})
	}
}

func TestKeyRingOpenNKRejectsUnknownKeyAndGarbage(t *testing.T) {
	ring, err := NewKeyRing(testStaticKey(t, "current"))
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}
	stranger := testStaticKey(t, "stranger")
	if _, _, _, err := ring.OpenNK(nkMessage1(t, stranger.Pair.Public, "x"), ""); !errors.Is(err, errKeyRingNoMatch) {
		t.Fatalf("foreign key error = %v", err)
	}
	if _, _, _, err := ring.OpenNK([]byte("not a handshake"), ""); !errors.Is(err, errKeyRingNoMatch) {
		t.Fatalf("garbage error = %v", err)
	}
	var nilRing *KeyRing
	if _, _, _, err := nilRing.OpenNK([]byte("x"), ""); !errors.Is(err, errKeyRingEmpty) {
		t.Fatalf("nil ring error = %v", err)
	}
	if nilRing.Len() != 0 {
		t.Fatal("nil ring Len != 0")
	}
	if _, ok := nilRing.Lookup("current"); ok {
		t.Fatal("nil ring Lookup = true")
	}
}
