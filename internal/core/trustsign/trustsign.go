// Package trustsign implements the detached-payload Ed25519 signing contract
// shared with Haruki-Client.
//
// Documents are never signed as parsed JSON: the signer signs the exact bytes
// it publishes, and the verifier checks the signature over those same bytes
// before decoding them. This sidesteps cross-language canonicalisation.
//
// Signing input is domain separated:
//
//	sign( domain || 0x00 || payload )
//
// so a signature over one document type can never be replayed as another.
package trustsign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// Algorithm is the only signature algorithm this contract supports.
	Algorithm = "ed25519"

	// DomainKeyset covers the offline-signed trust keyset (Noise public keys,
	// manifest signing keys, endpoint allowlist).
	DomainKeyset = "haruki-cloud/keyset/v1"
	// DomainManifest covers the command manifest served by the Cloud.
	DomainManifest = "haruki-cloud/manifest/v1"

	// EncodingJSON and EncodingMsgPack tell the verifier how to decode Payload
	// once the signature has been checked.
	EncodingJSON    = "json"
	EncodingMsgPack = "msgpack"

	// SeedSize is the Ed25519 private seed length in bytes.
	SeedSize = ed25519.SeedSize
)

var (
	ErrUnsupportedAlgorithm = errors.New("trustsign: unsupported signature algorithm")
	ErrDomainMismatch       = errors.New("trustsign: signature domain mismatch")
	ErrBadSignature         = errors.New("trustsign: signature verification failed")
	ErrBadPublicKey         = errors.New("trustsign: public key must be 32 bytes")
	ErrBadSeed              = errors.New("trustsign: private seed must be 32 bytes")
	ErrEmptyKeyID           = errors.New("trustsign: key id must not be empty")
	ErrBadDomain            = errors.New("trustsign: domain must be non-empty and free of NUL bytes")
)

// Envelope is the signed document as published on the wire. Payload and
// Signature are raw bytes; encoding/json renders them as base64.
type Envelope struct {
	Algorithm string `json:"alg" msgpack:"alg"`
	Domain    string `json:"domain" msgpack:"domain"`
	KeyID     string `json:"key_id" msgpack:"key_id"`
	Encoding  string `json:"encoding" msgpack:"encoding"`
	Payload   []byte `json:"payload" msgpack:"payload"`
	Signature []byte `json:"signature" msgpack:"signature"`
}

// SigningInput returns domain || 0x00 || payload.
func SigningInput(domain string, payload []byte) ([]byte, error) {
	if domain == "" || strings.ContainsRune(domain, 0) {
		return nil, ErrBadDomain
	}
	input := make([]byte, 0, len(domain)+1+len(payload))
	input = append(input, domain...)
	input = append(input, 0)
	input = append(input, payload...)
	return input, nil
}

// Signer holds one Ed25519 private key and its stable identifier.
type Signer struct {
	keyID string
	priv  ed25519.PrivateKey
}

// GenerateSeed returns a fresh random Ed25519 seed.
func GenerateSeed() ([]byte, error) {
	seed := make([]byte, SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	return seed, nil
}

// NewSigner builds a signer from a 32-byte seed.
func NewSigner(keyID string, seed []byte) (*Signer, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, ErrEmptyKeyID
	}
	if len(seed) != SeedSize {
		return nil, ErrBadSeed
	}
	return &Signer{keyID: keyID, priv: ed25519.NewKeyFromSeed(seed)}, nil
}

// NewSignerFromHex builds a signer from a hex-encoded 32-byte seed.
func NewSignerFromHex(keyID string, seedHex string) (*Signer, error) {
	seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
	if err != nil {
		return nil, fmt.Errorf("trustsign: decode seed hex: %w", err)
	}
	return NewSigner(keyID, seed)
}

// KeyID returns the signer's identifier.
func (s *Signer) KeyID() string { return s.keyID }

// PublicKey returns the Ed25519 public key.
func (s *Signer) PublicKey() ed25519.PublicKey {
	return s.priv.Public().(ed25519.PublicKey)
}

// PublicKeyHex returns the hex-encoded public key.
func (s *Signer) PublicKeyHex() string {
	return hex.EncodeToString(s.PublicKey())
}

// Sign wraps payload in a signed Envelope for the given domain. The payload
// bytes are copied so later mutation by the caller cannot desynchronise the
// envelope from its signature.
func (s *Signer) Sign(domain string, encoding string, payload []byte) (Envelope, error) {
	input, err := SigningInput(domain, payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Algorithm: Algorithm,
		Domain:    domain,
		KeyID:     s.keyID,
		Encoding:  encoding,
		Payload:   bytes.Clone(payload),
		Signature: ed25519.Sign(s.priv, input),
	}, nil
}

// ParsePublicKeyHex decodes a hex-encoded Ed25519 public key.
func ParsePublicKeyHex(pubHex string) (ed25519.PublicKey, error) {
	pub, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil {
		return nil, fmt.Errorf("trustsign: decode public key hex: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, ErrBadPublicKey
	}
	return ed25519.PublicKey(pub), nil
}

// Verify checks env against pub. wantDomain must match the envelope's domain
// exactly; an empty wantDomain skips that check (the signature still binds
// the domain, so a forged domain fails verification anyway).
func Verify(pub ed25519.PublicKey, env Envelope, wantDomain string) error {
	if len(pub) != ed25519.PublicKeySize {
		return ErrBadPublicKey
	}
	if env.Algorithm != Algorithm {
		return ErrUnsupportedAlgorithm
	}
	if wantDomain != "" && env.Domain != wantDomain {
		return ErrDomainMismatch
	}
	input, err := SigningInput(env.Domain, env.Payload)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, input, env.Signature) {
		return ErrBadSignature
	}
	return nil
}
