package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const (
	aesKeySize   = 32
	aesNonceSize = 12
	aesTagSize   = 16
)

var (
	errInvalidAESKeySize     = errors.New("invalid key size: must be 32 bytes for AES-256")
	errAESCiphertextTooShort = errors.New("ciphertext too short")
	errAESDecryptionFailed   = errors.New("decryption failed: authentication error")
)

// EncryptRaw encrypts plaintext with AES-256-GCM and returns raw bytes: nonce(12) || ciphertext || tag.
func EncryptRaw(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) != aesKeySize {
		return nil, errInvalidAESKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, aesNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	result := make([]byte, aesNonceSize+len(ciphertext))
	copy(result[:aesNonceSize], nonce)
	copy(result[aesNonceSize:], ciphertext)
	return result, nil
}

// DecryptRaw decrypts raw bytes (nonce(12) || ciphertext || tag) with AES-256-GCM.
func DecryptRaw(data []byte, key []byte) ([]byte, error) {
	if len(key) != aesKeySize {
		return nil, errInvalidAESKeySize
	}
	if len(data) < aesNonceSize+aesTagSize {
		return nil, errAESCiphertextTooShort
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := data[:aesNonceSize]
	ciphertext := data[aesNonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errAESDecryptionFailed
	}
	return plaintext, nil
}
