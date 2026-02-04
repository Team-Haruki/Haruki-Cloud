package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"io"
)

// KeyPair holds a Curve25519 key pair for ECDH.
type KeyPair struct {
	PrivateKey *ecdh.PrivateKey
	PublicKey  *ecdh.PublicKey
}

// GenerateKeyPair generates a new X25519 key pair.
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		PrivateKey: priv,
		PublicKey:  priv.PublicKey(),
	}, nil
}

// DeriveSharedSecret derives a shared secret using the private key and the peer's public key.
// It returns the raw shared secret bytes.
func DeriveSharedSecret(priv *ecdh.PrivateKey, peerPubBytes []byte) ([]byte, error) {
	peerPub, err := ecdh.X25519().NewPublicKey(peerPubBytes)
	if err != nil {
		return nil, err
	}
	return priv.ECDH(peerPub)
}

// EncryptAESGCM encrypts plaintext using AES-256-GCM with the given key.
// It generates a random nonce and prepends it to the ciphertext.
// The key must be 32 bytes long (AES-256).
func EncryptAESGCM(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptAESGCM decrypts ciphertext using AES-256-GCM with the given key.
// It expects the nonce to be prepended to the ciphertext.
func DecryptAESGCM(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, encryptedMsg := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encryptedMsg, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
