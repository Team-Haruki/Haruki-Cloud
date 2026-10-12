package trustsign

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// KeysetDocument is the payload of the offline-signed trust keyset. The client
// pins the offline root public key, verifies the envelope, then trusts every
// key and endpoint listed here. It is the only channel through which new Noise
// static keys, manifest signing keys or release signing keys reach a verifier.
type KeysetDocument struct {
	// Version must increase on every publication; clients reject rollbacks.
	Version int64 `json:"version"`
	// IssuedAt / ExpiresAt are unix seconds. Clients refuse expired keysets
	// and fall back to their built-in defaults.
	IssuedAt  int64 `json:"issued_at"`
	ExpiresAt int64 `json:"expires_at"`
	// NoiseKeys lists the Cloud Noise NK static public keys clients may
	// handshake against, current and next.
	NoiseKeys []KeysetKey `json:"noise_keys"`
	// ManifestSigningKeys lists the online Ed25519 keys the Cloud uses to
	// sign command manifests (two-tier delegation from the offline root).
	ManifestSigningKeys []KeysetKey `json:"manifest_signing_keys"`
	// ReleaseSigningKeys lists the Ed25519 keys delegated to sign client
	// build policies (DomainRelease) so routine releases do not need the
	// root seed. Every entry must carry not_after; the Cloud only honours a
	// key inside its window and while the keyset itself is unexpired.
	// Clients ignore this list.
	ReleaseSigningKeys []KeysetKey `json:"release_signing_keys,omitempty"`
	// Endpoints is the allowlist of Cloud base URLs by profile name.
	Endpoints map[string]string `json:"endpoints,omitempty"`
	// MinimumClientVersion lets the operator fence off old builds.
	MinimumClientVersion string `json:"minimum_client_version,omitempty"`
}

// KeysetKey names one public key with an optional validity window.
type KeysetKey struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"` // hex, 32 bytes
	NotBefore int64  `json:"not_before,omitempty"`
	NotAfter  int64  `json:"not_after,omitempty"`
}

// ValidAt reports whether now falls inside the key's validity window; a zero
// bound is open.
func (k KeysetKey) ValidAt(now time.Time) bool {
	unix := now.Unix()
	if k.NotBefore != 0 && unix < k.NotBefore {
		return false
	}
	if k.NotAfter != 0 && unix > k.NotAfter {
		return false
	}
	return true
}

var (
	ErrKeysetVersion      = errors.New("trustsign: keyset version must be positive")
	ErrKeysetValidity     = errors.New("trustsign: keyset expires_at must be after issued_at")
	ErrKeysetNoNoise      = errors.New("trustsign: keyset must list at least one noise key")
	ErrKeysetNotSigned    = errors.New("trustsign: envelope is not a signed keyset")
	ErrReleaseKeyNoExpiry = errors.New("trustsign: release signing keys must set not_after")
)

// Validate checks structural sanity: positive version, coherent validity
// window, at least one Noise key, and well-formed unique keys in every list.
func (d KeysetDocument) Validate() error {
	if d.Version <= 0 {
		return ErrKeysetVersion
	}
	if d.IssuedAt <= 0 || d.ExpiresAt <= d.IssuedAt {
		return ErrKeysetValidity
	}
	if len(d.NoiseKeys) == 0 {
		return ErrKeysetNoNoise
	}
	if err := validateKeysetKeys("noise_keys", d.NoiseKeys); err != nil {
		return err
	}
	if err := validateKeysetKeys("manifest_signing_keys", d.ManifestSigningKeys); err != nil {
		return err
	}
	if err := validateKeysetKeys("release_signing_keys", d.ReleaseSigningKeys); err != nil {
		return err
	}
	for i, key := range d.ReleaseSigningKeys {
		if key.NotAfter == 0 {
			return fmt.Errorf("trustsign: release_signing_keys[%d] (%s): %w", i, key.KeyID, ErrReleaseKeyNoExpiry)
		}
	}
	for name, url := range d.Endpoints {
		if strings.TrimSpace(name) == "" || !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("trustsign: endpoint %q must be an https URL", name)
		}
	}
	return nil
}

// ExpiredAt reports whether the keyset is past its expires_at.
func (d KeysetDocument) ExpiredAt(now time.Time) bool {
	return now.Unix() >= d.ExpiresAt
}

// ReleaseKey returns the release signing key with the given id, if listed.
// Validity is the caller's concern (ValidAt plus the keyset's own expiry).
func (d KeysetDocument) ReleaseKey(keyID string) (KeysetKey, bool) {
	for _, key := range d.ReleaseSigningKeys {
		if key.KeyID == keyID {
			return key, true
		}
	}
	return KeysetKey{}, false
}

// ParseKeyset decodes a signed keyset envelope, verifies it against rootPub
// and validates the document. It is what the Cloud uses to trust the
// delegated keys the keyset lists; the signature is mandatory here, unlike
// the serving path which republishes the file verbatim.
func ParseKeyset(raw []byte, rootPub ed25519.PublicKey) (*KeysetDocument, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("trustsign: decode keyset envelope: %w", err)
	}
	if env.Algorithm != Algorithm || env.Domain != DomainKeyset || len(env.Signature) == 0 || len(env.Payload) == 0 {
		return nil, ErrKeysetNotSigned
	}
	if err := Verify(rootPub, env, DomainKeyset); err != nil {
		return nil, fmt.Errorf("trustsign: keyset %w", err)
	}
	var doc KeysetDocument
	if err := json.Unmarshal(env.Payload, &doc); err != nil {
		return nil, fmt.Errorf("trustsign: decode keyset payload: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

func validateKeysetKeys(field string, keys []KeysetKey) error {
	seen := make(map[string]struct{}, len(keys))
	for i, key := range keys {
		if strings.TrimSpace(key.KeyID) == "" {
			return fmt.Errorf("trustsign: %s[%d]: %w", field, i, ErrEmptyKeyID)
		}
		if _, dup := seen[key.KeyID]; dup {
			return fmt.Errorf("trustsign: %s[%d]: duplicate key id %q", field, i, key.KeyID)
		}
		seen[key.KeyID] = struct{}{}
		if _, err := ParsePublicKeyHex(key.PublicKey); err != nil {
			return fmt.Errorf("trustsign: %s[%d] (%s): %w", field, i, key.KeyID, err)
		}
		if key.NotAfter != 0 && key.NotBefore != 0 && key.NotAfter <= key.NotBefore {
			return fmt.Errorf("trustsign: %s[%d] (%s): not_after must be after not_before", field, i, key.KeyID)
		}
	}
	return nil
}
