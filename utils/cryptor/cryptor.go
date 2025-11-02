package cryptor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

type HarukiCryptor struct {
	key []byte
}

func NewHarukiCryptor(key []byte) (*HarukiCryptor, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("invalid key size: must be 16, 24, or 32 bytes")
	}
	return &HarukiCryptor{key: key}, nil
}

func (h *HarukiCryptor) Pack(content interface{}) ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	block, err := aes.NewCipher(h.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	data, err := msgpack.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}
	ciphertextWithTag := aesGCM.Seal(nil, nonce, data, nil)
	tagSize := aesGCM.Overhead()
	if len(ciphertextWithTag) < tagSize {
		return nil, fmt.Errorf("encrypted data too short")
	}
	ciphertext := ciphertextWithTag[:len(ciphertextWithTag)-tagSize]
	tag := ciphertextWithTag[len(ciphertextWithTag)-tagSize:]
	result := make([]byte, 0, len(nonce)+len(tag)+len(ciphertext))
	result = append(result, nonce...)
	result = append(result, tag...)
	result = append(result, ciphertext...)
	return result, nil
}

func (h *HarukiCryptor) Unpack(content []byte) (interface{}, error) {
	if len(content) < 28 { // 12 (nonce) + 16 (tag) = 28 minimum
		return nil, fmt.Errorf("invalid content: too short (minimum 28 bytes required)")
	}
	nonce := content[:12]
	tag := content[12:28]
	ciphertext := content[28:]
	block, err := aes.NewCipher(h.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	ciphertextWithTag := append(ciphertext, tag...)
	decryptedData, err := aesGCM.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	var result interface{}
	if err := msgpack.Unmarshal(decryptedData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	return result, nil
}
