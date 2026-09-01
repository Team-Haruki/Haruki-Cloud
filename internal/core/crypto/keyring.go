package crypto

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
)

// DefaultKeyID names a Noise static key configured through the legacy
// single-key setting, which carries no explicit identifier.
const DefaultKeyID = "default"

var (
	errKeyRingEmpty      = errors.New("noise key ring requires at least one static key")
	errKeyRingNoMatch    = errors.New("noise handshake did not match any configured static key")
	errStaticKeyMissing  = errors.New("noise static key requires a key pair")
	errStaticKeyNoID     = errors.New("noise static key requires a non-empty key id")
	errStaticKeyBadID    = errors.New("noise static key id must not contain whitespace")
	errStaticKeyDupID    = errors.New("duplicate noise static key id")
	errStaticKeyDupPriv  = errors.New("duplicate noise static private key")
	errStaticKeyBadPriv  = errors.New("noise static private key must be 32 bytes")
	errStaticKeyBadPubKy = errors.New("noise static public key must be 32 bytes")
)

// StaticKey is a Noise static key pair with a stable identifier that clients
// may reference when selecting which server public key to handshake against.
type StaticKey struct {
	ID   string
	Pair *KeyPair
}

// KeyRing is an ordered set of Noise static key pairs. The first entry is the
// primary key advertised to clients; the remaining entries stay accepted so
// clients can migrate during a key rotation without an outage.
type KeyRing struct {
	keys []StaticKey
}

// NewKeyRing validates the given keys and builds a ring preserving their order.
func NewKeyRing(keys ...StaticKey) (*KeyRing, error) {
	if len(keys) == 0 {
		return nil, errKeyRingEmpty
	}
	seenID := make(map[string]struct{}, len(keys))
	ring := &KeyRing{keys: make([]StaticKey, 0, len(keys))}
	for i, key := range keys {
		if err := validateStaticKey(key); err != nil {
			return nil, fmt.Errorf("noise key %d: %w", i, err)
		}
		if _, dup := seenID[key.ID]; dup {
			return nil, fmt.Errorf("noise key %d (%s): %w", i, key.ID, errStaticKeyDupID)
		}
		for _, existing := range ring.keys {
			if subtle.ConstantTimeCompare(existing.Pair.Private, key.Pair.Private) == 1 {
				return nil, fmt.Errorf("noise key %d (%s): %w", i, key.ID, errStaticKeyDupPriv)
			}
		}
		seenID[key.ID] = struct{}{}
		ring.keys = append(ring.keys, key)
	}
	return ring, nil
}

// SingleKeyRing wraps one key pair under DefaultKeyID.
func SingleKeyRing(pair *KeyPair) (*KeyRing, error) {
	return NewKeyRing(StaticKey{ID: DefaultKeyID, Pair: pair})
}

func validateStaticKey(key StaticKey) error {
	switch {
	case key.Pair == nil:
		return errStaticKeyMissing
	case len(key.Pair.Private) != 32:
		return errStaticKeyBadPriv
	case len(key.Pair.Public) != 32:
		return errStaticKeyBadPubKy
	case key.ID == "":
		return errStaticKeyNoID
	case strings.ContainsAny(key.ID, " \t\r\n"):
		return errStaticKeyBadID
	}
	return nil
}

// Primary returns the first key in the ring.
func (r *KeyRing) Primary() StaticKey {
	return r.keys[0]
}

// Keys returns a copy of the ring in priority order.
func (r *KeyRing) Keys() []StaticKey {
	out := make([]StaticKey, len(r.keys))
	copy(out, r.keys)
	return out
}

// Len reports how many static keys the ring holds.
func (r *KeyRing) Len() int {
	if r == nil {
		return 0
	}
	return len(r.keys)
}

// Lookup finds a key by identifier.
func (r *KeyRing) Lookup(id string) (StaticKey, bool) {
	if r == nil {
		return StaticKey{}, false
	}
	for _, key := range r.keys {
		if key.ID == id {
			return key, true
		}
	}
	return StaticKey{}, false
}

// OpenNK consumes a Noise NK message 1 against the ring. The key named by
// preferredID is tried first when present; every other key is then tried in
// ring order. On success it returns the responder ready to encrypt message 2,
// the decrypted payload, and the id of the key that matched.
//
// Trying each key is safe because the NK handshake authenticates the
// responder's static key inside the AEAD tag: a message built for a different
// static key fails decryption instead of yielding garbage.
func (r *KeyRing) OpenNK(ciphertext []byte, preferredID string) (*NoiseCipher, []byte, string, error) {
	if r == nil || len(r.keys) == 0 {
		return nil, nil, "", errKeyRingEmpty
	}
	order := make([]StaticKey, 0, len(r.keys))
	if preferred, ok := r.Lookup(preferredID); ok {
		order = append(order, preferred)
	}
	for _, key := range r.keys {
		if key.ID != preferredID {
			order = append(order, key)
		}
	}
	var lastErr error
	for _, key := range order {
		responder, err := NewResponder(key.Pair)
		if err != nil {
			return nil, nil, "", err
		}
		plaintext, err := responder.DecryptPacket(ciphertext)
		if err == nil {
			return responder, plaintext, key.ID, nil
		}
		lastErr = err
	}
	return nil, nil, "", fmt.Errorf("%w: %w", errKeyRingNoMatch, lastErr)
}
