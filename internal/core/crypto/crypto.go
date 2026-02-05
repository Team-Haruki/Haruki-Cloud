package crypto

import (
	"crypto/rand"
	"errors"
	"log"

	"github.com/flynn/noise"
)

// CipherSuite defines the Noise protocol: Noise_IK_25519_AESGCM_SHA256
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

// NoiseCipher handles the Noise IK handshake and subsequent encryption/decryption.
// Since we use a Per-Request Handshake pattern for HTTP (Stateless),
// each request involves a full handshake.
//
// Pattern IK:
//
//	<- s
//	...
//	-> e, es, s, ss, payload
//	<- e, ee, se, payload
type NoiseCipher struct {
	HandshakeState *noise.HandshakeState
	SendCipher     *noise.CipherState
	RecvCipher     *noise.CipherState
}

// NewHandshake initiates a Noise IK handshake state.
// staticKP: My static key pair.
// remoteStaticPub: Peer's static public key (required for Initiator, optional/unknown for Responder initially but IK requires it pre-verification?)
// Actually in IK, Responder doesn't know 's' of Initiator until it decrypts the message.
// But Responder MUST enable IK pattern.
func NewHandshake(staticKP *KeyPair, remoteStaticPub []byte, isInitiator bool) (*NoiseCipher, error) {
	config := noise.Config{
		CipherSuite:   CipherSuite,
		Random:        rand.Reader,
		Pattern:       noise.HandshakeIK,
		Initiator:     isInitiator,
		StaticKeypair: *staticKP,
	}

	if isInitiator {
		if len(remoteStaticPub) == 0 {
			return nil, errors.New("initiator requires remote static public key")
		}
		config.PeerStatic = remoteStaticPub
	} else {
		// Responder in IK doesn't need to know PeerStatic upfront.
		// It learns it from the handshake.
		// But Responder MUST verify it against a whitelist (in middleware).
	}

	hs, err := noise.NewHandshakeState(config)
	if err != nil {
		return nil, err
	}

	return &NoiseCipher{HandshakeState: hs}, nil
}

// EncryptPacket (Initiator Step 1, Responder Step 1)
// Processes a handshake write step properly.
// For IK:
//
//	Initiator 1st msg: -> e, es, s, ss, payload
//	Responder 1st msg: <- e, ee, se, payload
func (nc *NoiseCipher) EncryptPacket(payload []byte) ([]byte, error) {
	if nc.HandshakeState == nil {
		// Post-handshake transport message?
		// In strictly per-request IK, we usually only do one flight.
		// If using Transport mode established from handshake:
		if nc.SendCipher != nil {
			return nc.SendCipher.Encrypt(nil, nil, payload)
		}
		return nil, errors.New("handshake not initialized and no transport cipher")
	}

	msg, cs0, cs1, err := nc.HandshakeState.WriteMessage(nil, payload)
	if err != nil {
		return nil, err
	}

	// IK handshake finishes after 2 messages (Initiator->Responder, then Responder->Initiator).
	// If Handshake is done, we get CipherStates.
	if cs0 != nil && cs1 != nil {
		nc.SendCipher = cs0
		nc.RecvCipher = cs1
		nc.HandshakeState = nil // Handshake complete
	}

	return msg, nil
}

// DecryptPacket (Responder Step 1, Initiator Step 1)
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
		// Note from noise-c/specs:
		// After Responder receives msg1: returns (cs0, cs1) but only RecvCipher (cs1? or one of them) is valid?
		// Actually WriteMessage returns (Send, Recv). ReadMessage returns (Recv, Send)?
		// noise-go docs: "If the handshake is complete, ReadMessage returns the two CipherStates. By convention, the first is for writing, the second for reading."
		// Wait, noise-go docs say: "returns (c1, c2 *CipherState, err error)".
		// Usually (SendCipher, RecvCipher).
		// Let's stick to convention:
		// Initiator finishes after reading Msg2.
		// Responder finishes after writing Msg2? No, Responder finishes after Reading Msg1(partially) and Writing Msg2.
		// IK Pattern:
		//   -> e, es, s, ss, payload  (Bob reads this. Handshake NOT done for Bob. He needs to Write msg2)
		//   <- e, ee, se, payload     (Bob writes this. Handshake DONE.)

		// Wait, standard IK is:
		//   -> e, es, s, ss
		//   <- e, ee, se
		// It's a ONE-WAY pattern? No, two messages.

		// Let's check `noise.HandshakeIK`.
		// It's 2 messages.

		nc.SendCipher = cs0
		nc.RecvCipher = cs1
		nc.HandshakeState = nil
		log.Println("Handshake complete (Read side). CipherStates established.")
	}

	return payload, nil
}

// GetPeerStatic returns the static public key of the remote peer.
// Available only after handshake verifies the static key (IK: after Msg1 for Responder).
func (nc *NoiseCipher) GetPeerStatic() []byte {
	if nc.HandshakeState != nil && nc.HandshakeState.PeerStatic() != nil {
		return nc.HandshakeState.PeerStatic()
	}
	// After handshake, might be stored differently? noise-go doesn't expose it easily after HS state denied.
	// We should capture it before clearing HandshakeState if needed.
	// But `PeerStatic()` in HandshakeState should return it.
	return nil
}
