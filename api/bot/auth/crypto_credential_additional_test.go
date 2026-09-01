package auth

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCredentialVerificationBranches(t *testing.T) {
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
