package secure

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haruki-cloud/internal/core/crypto"

	"github.com/gofiber/fiber/v3"
)

func secureTestKeyPair(t *testing.T) *crypto.KeyPair {
	t.Helper()
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return keyPair
}

func secureTestRequest(t *testing.T, app *fiber.App, keyPair *crypto.KeyPair, payload []byte) (int, []byte, string) {
	t.Helper()
	initiator, err := crypto.NewInitiator(keyPair.Public)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	ciphertext, err := initiator.EncryptPacket(payload)
	if err != nil {
		t.Fatalf("encrypt request: %v", err)
	}
	request := httptest.NewRequest("POST", "/secure", bytes.NewReader(ciphertext))
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	plaintext, err := initiator.DecryptPacket(body)
	if err != nil {
		t.Fatalf("decrypt response: %v (wire body %q)", err, body)
	}
	return response.StatusCode, plaintext, response.Header.Get("Content-Type")
}

func TestSecureMiddlewareRoundTrip(t *testing.T) {
	keyPair := secureTestKeyPair(t)
	app := fiber.New()
	app.Post("/secure", New(Config{ServerPrivateKey: keyPair}), func(c fiber.Ctx) error {
		if got := string(c.Body()); got != "request payload" {
			t.Fatalf("decrypted request body = %q", got)
		}
		if got := c.Get("Content-Type"); got != "application/msgpack" {
			t.Fatalf("request content type = %q", got)
		}
		if secure, ok := c.Locals("secure_noise").(bool); !ok || !secure {
			t.Fatalf("secure_noise local = %#v", c.Locals("secure_noise"))
		}
		return c.SendString("response payload")
	})

	status, body, contentType := secureTestRequest(t, app, keyPair, []byte("request payload"))
	if status != fiber.StatusOK || string(body) != "response payload" || contentType != "application/octet-stream" {
		t.Fatalf("secure response = status %d, type %q, body %q", status, contentType, body)
	}
}

func TestSecureMiddlewareEncryptsDownstreamErrorsAndPanics(t *testing.T) {
	keyPair := secureTestKeyPair(t)
	for name, handler := range map[string]fiber.Handler{
		"error": func(fiber.Ctx) error { return fiber.ErrTeapot },
		"panic": func(fiber.Ctx) error { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			app := fiber.New()
			app.Post("/secure", New(Config{ServerPrivateKey: keyPair}), handler)
			status, body, contentType := secureTestRequest(t, app, keyPair, []byte("request"))
			wantStatus := fiber.StatusTeapot
			if name == "panic" {
				wantStatus = fiber.StatusInternalServerError
			}
			if status != wantStatus || len(body) == 0 || contentType != "application/octet-stream" {
				t.Fatalf("secure %s response = status %d, type %q, body %q", name, status, contentType, body)
			}
		})
	}
}

func TestSecureMiddlewareErrorHandlerFallbackIsEncrypted(t *testing.T) {
	keyPair := secureTestKeyPair(t)
	app := fiber.New(fiber.Config{
		ErrorHandler: func(fiber.Ctx, error) error {
			return errors.New("error handler failed")
		},
	})
	app.Post("/secure", New(Config{ServerPrivateKey: keyPair}), func(fiber.Ctx) error {
		return fiber.ErrBadRequest
	})

	status, body, _ := secureTestRequest(t, app, keyPair, []byte("request"))
	if status != fiber.StatusInternalServerError || string(body) != "Internal Server Error" {
		t.Fatalf("fallback response = status %d, body %q", status, body)
	}
}

func TestSecureMiddlewareRejectsPlaintextRequests(t *testing.T) {
	keyPair := secureTestKeyPair(t)
	app := fiber.New()
	app.Post("/secure", New(Config{ServerPrivateKey: keyPair}), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	for name, body := range map[string][]byte{
		"empty":   nil,
		"invalid": []byte("not a Noise handshake"),
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/secure", bytes.NewReader(body))
			response, err := app.Test(request)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			responseBody, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if response.StatusCode != fiber.StatusBadRequest || !strings.Contains(string(responseBody), "error") {
				t.Fatalf("plaintext response = status %d, body %q", response.StatusCode, responseBody)
			}
		})
	}
}

func TestSecureMiddlewareRequiresServerKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New did not panic for a nil server key")
		}
	}()
	_ = New(Config{})
}

// secureRingRequest encrypts a fixed payload for pair, optionally sending a
// key-id hint, and returns the raw response together with the initiator so
// the caller can decrypt the body.
func secureRingRequest(t *testing.T, app *fiber.App, pair *crypto.KeyPair, hint string) (*http.Response, []byte, *crypto.NoiseCipher) {
	t.Helper()
	initiator, err := crypto.NewInitiator(pair.Public)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	ciphertext, err := initiator.EncryptPacket([]byte("payload"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	request := httptest.NewRequest("POST", "/secure", bytes.NewReader(ciphertext))
	if hint != "" {
		request.Header.Set(HeaderNoiseKeyID, hint)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	return response, body, initiator
}

func TestSecureMiddlewareAcceptsEveryKeyInRing(t *testing.T) {
	current := secureTestKeyPair(t)
	next := secureTestKeyPair(t)
	ring, err := crypto.NewKeyRing(
		crypto.StaticKey{ID: "current", Pair: current},
		crypto.StaticKey{ID: "next", Pair: next},
	)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}

	var seenKeyID string
	app := fiber.New()
	app.Post("/secure", New(Config{KeyRing: ring}), func(c fiber.Ctx) error {
		seenKeyID, _ = c.Locals(LocalNoiseKeyID).(string)
		return c.SendString("ok")
	})

	for _, tc := range []struct {
		name   string
		pair   *crypto.KeyPair
		hint   string
		wantID string
	}{
		{name: "primary", pair: current, wantID: "current"},
		{name: "rotation key without hint", pair: next, wantID: "next"},
		{name: "rotation key with hint", pair: next, hint: "next", wantID: "next"},
		{name: "rotation key with wrong hint", pair: next, hint: "current", wantID: "next"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seenKeyID = ""
			response, body, initiator := secureRingRequest(t, app, tc.pair, tc.hint)
			if response.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d body %q", response.StatusCode, body)
			}
			if plaintext, err := initiator.DecryptPacket(body); err != nil || string(plaintext) != "ok" {
				t.Fatalf("decrypt response = %q, %v", plaintext, err)
			}
			if seenKeyID != tc.wantID || response.Header.Get(HeaderNoiseKeyID) != tc.wantID {
				t.Fatalf("key id local %q header %q, want %q", seenKeyID, response.Header.Get(HeaderNoiseKeyID), tc.wantID)
			}
		})
	}

	// A key outside the ring must be refused before reaching the handler.
	response, _, _ := secureRingRequest(t, app, secureTestKeyPair(t), "")
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("foreign key status = %d", response.StatusCode)
	}
}

func TestSecureMiddlewarePanicsWithoutKeys(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("New(Config{}) did not panic")
		}
	}()
	New(Config{})
}
