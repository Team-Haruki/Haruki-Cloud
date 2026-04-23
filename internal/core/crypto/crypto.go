package crypto

import (
	"crypto/rand"
	"errors"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// CipherSuite defines the Noise protocol: Noise_NK_25519_AESGCM_SHA256
var CipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherAESGCM, noise.HashSHA256)

// KeyPair holds X25519 keys.
type KeyPair = noise.DHKey

// GenerateKeyPair generates a new X25519 key pair using Noise library.
func GenerateKeyPair() (*KeyPair, error) {
	kp, err := CipherSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &kp, nil
}

// KeyPairFromPrivate derives the X25519 public key from a 32-byte private key
// and returns a complete KeyPair suitable for use with the Noise NK handshake.
func KeyPairFromPrivate(private []byte) (*KeyPair, error) {
	if len(private) != 32 {
		return nil, errors.New("private key must be 32 bytes")
	}
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return nil, err
	}
	return &KeyPair{Private: private, Public: public}, nil
}

// NoiseCipher handles the Noise NK handshake and subsequent encryption/decryption.
// Since we use a Per-Request Handshake pattern for HTTP (Stateless),
// each request involves a full handshake.
//
// Pattern NK:
//
//	<- s
//	...
//	-> e, es, payload
//	<- e, ee, payload
type NoiseCipher struct {
	HandshakeState *noise.HandshakeState
	SendCipher     *noise.CipherState
	RecvCipher     *noise.CipherState
}

// NewResponder creates a Noise NK responder (server side).
// The server provides its static key pair; no knowledge of the client is needed.
func NewResponder(staticKP *KeyPair) (*NoiseCipher, error) {
	if staticKP == nil {
		return nil, errors.New("responder requires static key pair")
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   CipherSuite,
		Random:        rand.Reader,
		Pattern:       noise.HandshakeNK,
		Initiator:     false,
		StaticKeypair: *staticKP,
	})
	if err != nil {
		return nil, err
	}
	return &NoiseCipher{HandshakeState: hs}, nil
}

// NewInitiator creates a Noise NK initiator (client side).
// The client only needs the server's static public key; no client key pair is required.
func NewInitiator(serverPubKey []byte) (*NoiseCipher, error) {
	if len(serverPubKey) == 0 {
		return nil, errors.New("initiator requires server static public key")
	}
	// NK initiator has no static key — generate an empty one that won't be used.
	// The NK pattern never transmits 's' from initiator, so this is safe.
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite: CipherSuite,
		Random:      rand.Reader,
		Pattern:     noise.HandshakeNK,
		Initiator:   true,
		PeerStatic:  serverPubKey,
	})
	if err != nil {
		return nil, err
	}
	return &NoiseCipher{HandshakeState: hs}, nil
}

// EncryptPacket writes the next handshake message with the given payload.
// For NK:
//
//	Initiator msg 1: -> e, es, payload
//	Responder msg 2: <- e, ee, payload
func (nc *NoiseCipher) EncryptPacket(payload []byte) ([]byte, error) {
	if nc.HandshakeState == nil {
		if nc.SendCipher != nil {
			return nc.SendCipher.Encrypt(nil, nil, payload)
		}
		return nil, errors.New("handshake not initialized and no transport cipher")
	}

	msg, cs0, cs1, err := nc.HandshakeState.WriteMessage(nil, payload)
	if err != nil {
		return nil, err
	}

	if cs0 != nil && cs1 != nil {
		nc.SendCipher = cs0
		nc.RecvCipher = cs1
		nc.HandshakeState = nil
	}

	return msg, nil
}

// DecryptPacket reads the next handshake message and returns the decrypted payload.
func (nc *NoiseCipher) DecryptPacket(ciphertext []byte) ([]byte, error) {
	if nc.HandshakeState == nil {
		if nc.RecvCipher != nil {
			return nc.RecvCipher.Decrypt(nil, nil, ciphertext)
		}
		return nil, errors.New("handshake not initialized and no transport cipher")
	}

	payload, cs0, cs1, err := nc.HandshakeState.ReadMessage(nil, ciphertext)
	if err != nil {
		return nil, err
	}

	if cs0 != nil && cs1 != nil {
		nc.SendCipher = cs0
		nc.RecvCipher = cs1
		nc.HandshakeState = nil
	}

	return payload, nil
}
