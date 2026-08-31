package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"haruki-cloud/config"

	"github.com/shamaton/msgpack/v3"
	"golang.org/x/crypto/bcrypt"
)

func TestEncryptDecryptRawValidation(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	plaintext := []byte("haruki auth payload")
	encrypted, err := EncryptRaw(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptRaw() error = %v", err)
	}
	decrypted, err := DecryptRaw(encrypted, key)
	if err != nil || !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("DecryptRaw() = %q, %v", decrypted, err)
	}
	if _, err := EncryptRaw(plaintext, key[:16]); !errors.Is(err, errInvalidAESKeySize) {
		t.Fatalf("short encryption key error = %v", err)
	}
	if _, err := DecryptRaw(encrypted, key[:16]); !errors.Is(err, errInvalidAESKeySize) {
		t.Fatalf("short decryption key error = %v", err)
	}
	if _, err := DecryptRaw(make([]byte, aesNonceSize+aesTagSize-1), key); !errors.Is(err, errAESCiphertextTooShort) {
		t.Fatalf("short ciphertext error = %v", err)
	}
	tampered := append([]byte(nil), encrypted...)
	tampered[len(tampered)-1] ^= 1
	if _, err := DecryptRaw(tampered, key); !errors.Is(err, errAESDecryptionFailed) {
		t.Fatalf("tampered ciphertext error = %v", err)
	}
}

func TestCredentialAndSessionTTLBranches(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash credential: %v", err)
	}
	if !verifyCredential(string(hashed), "secret") || verifyCredential(string(hashed), "wrong") {
		t.Fatal("bcrypt credential verification returned the wrong result")
	}
	if !verifyCredential("legacy-secret", "legacy-secret") || verifyCredential("legacy-secret", "wrong") {
		t.Fatal("legacy credential verification returned the wrong result")
	}
	if verifyCredential("", "") || verifyCredential("legacy-secret", "") {
		t.Fatal("empty credentials were accepted")
	}

	previous := config.Cfg.HarukiBotDB.SessionTTLDays
	t.Cleanup(func() { config.Cfg.HarukiBotDB.SessionTTLDays = previous })
	config.Cfg.HarukiBotDB.SessionTTLDays = 0
	if got := getSessionTTL(); got != 7*24*time.Hour {
		t.Fatalf("default session TTL = %s", got)
	}
	config.Cfg.HarukiBotDB.SessionTTLDays = 31
	if got := getSessionTTL(); got != 30*24*time.Hour {
		t.Fatalf("capped session TTL = %s", got)
	}
	config.Cfg.HarukiBotDB.SessionTTLDays = 2
	if got := getSessionTTL(); got != 2*24*time.Hour {
		t.Fatalf("configured session TTL = %s", got)
	}
}

func TestPublicTelemetryDispatcherConstructor(t *testing.T) {
	if NewCommandTelemetryDispatcher(nil) != nil {
		t.Fatal("nil client created a dispatcher")
	}
	var nilDispatcher *CommandTelemetryDispatcher
	if !nilDispatcher.Enqueue(context.Background(), 1, CommandLogEntry{}) {
		t.Fatal("nil dispatcher should accept a no-op enqueue")
	}

	client := newBotTestClient(t, "public_telemetry_dispatcher")
	t.Cleanup(func() { _ = client.Close() })
	dispatcher := NewCommandTelemetryDispatcher(client)
	if dispatcher == nil {
		t.Fatal("non-nil client did not create a dispatcher")
	}
	dispatcher.Close()
	dispatcher.Close()
	if dispatcher.Enqueue(context.Background(), 1, CommandLogEntry{}) {
		t.Fatal("closed dispatcher accepted telemetry")
	}
}

func TestDecodeAuthPayloadValidation(t *testing.T) {
	ctx := context.Background()
	key := []byte("01234567890123456789012345678901")
	handler := NewUserHandler(NewUserServiceWithDependencies(nil, newMemoryRedisStore(), key, ""))
	now := time.Now().Unix()

	invalidMsgpack, err := EncryptRaw([]byte{0xc1}, key)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		botID   string
		body    []byte
		key     []byte
		message string
	}{
		{name: "missing key", botID: "1", body: []byte("body")},
		{name: "missing body", botID: "1", key: key, message: ErrInvalidEncryptedData},
		{name: "invalid ciphertext", botID: "1", body: []byte("body"), key: key, message: ErrInvalidEncryptedData},
		{name: "invalid msgpack", botID: "1", body: invalidMsgpack, key: key, message: ErrInvalidEncryptedData},
		{name: "bot mismatch", botID: "1", body: encryptedAuthPayload(t, key, HarukiAuthPayload{BotID: "2", Timestamp: now}), key: key, message: ErrBotIDMismatch},
		{name: "expired timestamp", botID: "1", body: encryptedAuthPayload(t, key, HarukiAuthPayload{BotID: "1", Timestamp: now - MaxAuthTimestampAge - 1}), key: key, message: ErrAuthTimestampExpired},
		{name: "future timestamp", botID: "1", body: encryptedAuthPayload(t, key, HarukiAuthPayload{BotID: "1", Timestamp: now + MaxAuthTimestampAge + 1}), key: key, message: ErrAuthTimestampExpired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, responseErr := handler.decodeAuthPayload(ctx, test.botID, test.body, test.key)
			if responseErr == nil || responseErr.message != test.message {
				t.Fatalf("decode error = %#v, want message %q", responseErr, test.message)
			}
		})
	}

	validBody := encryptedAuthPayload(t, key, HarukiAuthPayload{BotID: "valid", Timestamp: now})
	payload, responseErr := handler.decodeAuthPayload(ctx, "valid", validBody, key)
	if responseErr != nil || payload.BotID != "valid" {
		t.Fatalf("valid payload = %+v, error = %#v", payload, responseErr)
	}
	if _, responseErr := handler.decodeAuthPayload(ctx, "valid", validBody, key); responseErr == nil || responseErr.message != ErrReplayDetected {
		t.Fatalf("replayed payload error = %#v", responseErr)
	}
}

func encryptedAuthPayload(t *testing.T, key []byte, payload HarukiAuthPayload) []byte {
	t.Helper()
	plaintext, err := msgpack.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal auth payload: %v", err)
	}
	encrypted, err := EncryptRaw(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt auth payload: %v", err)
	}
	return encrypted
}
