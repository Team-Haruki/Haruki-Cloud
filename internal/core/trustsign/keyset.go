package trustsign

import (
	"errors"
	"fmt"
	"strings"
)

// KeysetDocument is the payload of the offline-signed trust keyset. The client
// pins the offline root public key, verifies the envelope, then trusts every
// key and endpoint listed here. It is the only channel through which new Noise
// static keys or manifest signing keys reach the client.
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

var (
	ErrKeysetVersion  = errors.New("trustsign: keyset version must be positive")
	ErrKeysetValidity = errors.New("trustsign: keyset expires_at must be after issued_at")
	ErrKeysetNoNoise  = errors.New("trustsign: keyset must list at least one noise key")
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
	for name, url := range d.Endpoints {
		if strings.TrimSpace(name) == "" || !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("trustsign: endpoint %q must be an https URL", name)
		}
	}
	return nil
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
