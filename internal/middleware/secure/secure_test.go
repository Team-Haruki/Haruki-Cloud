package secure

import (
	"bytes"
	"errors"
	"io"
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
