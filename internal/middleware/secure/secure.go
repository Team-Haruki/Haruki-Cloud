package secure

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/observability/commandtrace"

	"github.com/gofiber/fiber/v3"
)

// Config defines the config for Secure middleware.
type Config struct {
	// ServerPrivateKey is the server's static key pair used for Noise NK.
	ServerPrivateKey *crypto.KeyPair
}

// New creates a new Secure middleware with Noise NK transport encryption.
// Each HTTP request performs a full NK handshake:
//
//	Request body  = Noise NK Message 1 (-> e, es, payload)
//	Response body = Noise NK Message 2 (<- e, ee, payload)
func New(config Config) fiber.Handler {
	if config.ServerPrivateKey == nil {
		panic("secure middleware: ServerPrivateKey is required")
	}

	return func(c fiber.Ctx) error {
		sendJSON := func(status int, payload fiber.Map) error {
			finishResponse := commandtrace.MeasurePhase(c.Context(), "response_encode")
			defer finishResponse()
			return c.Status(status).JSON(payload)
		}
		finishDecrypt := commandtrace.MeasurePhase(c.Context(), "noise_decrypt")
		// 1. Initialize Noise NK Responder
		nc, err := crypto.NewResponder(config.ServerPrivateKey)
		if err != nil {
			finishDecrypt()
			commandtrace.SetErrorType(c.Context(), fmt.Sprintf("%T", err))
			return sendJSON(fiber.StatusInternalServerError, fiber.Map{"error": "Crypto init failed"})
		}

		// 2. Read Request Body (NK Message 1: -> e, es, payload)
		ciphertext := c.Body()
		if len(ciphertext) == 0 {
			finishDecrypt()
			return sendJSON(fiber.StatusBadRequest, fiber.Map{"error": "Empty body"})
		}

		plaintext, err := nc.DecryptPacket(ciphertext)
		if err != nil {
			finishDecrypt()
			commandtrace.SetErrorType(c.Context(), fmt.Sprintf("%T", err))
			slog.WarnContext(c.Context(), "noise handshake/decrypt failed", "error_type", fmt.Sprintf("%T", err))
			return sendJSON(fiber.StatusBadRequest, fiber.Map{"error": "Secure handshake failed (Decrypt)"})
		}
		finishDecrypt()

		// 3. Set Request Body to Plaintext
		c.Request().SetBody(plaintext)
		c.Request().Header.Set("Content-Type", "application/msgpack")
		c.Locals("secure_noise", true)

		// 4. Continue stack. Once the Noise handshake succeeds, downstream
		// errors and panics must be rendered here so the fallback response still
		// traverses the response-encryption half of this middleware.
		recovered, panicStack, downstreamErr := nextSecureHandler(c)
		switch {
		case recovered != nil:
			commandtrace.SetErrorType(c.Context(), "panic")
			attrs := []any{"panic_type", fmt.Sprintf("%T", recovered)}
			if !harukiConfig.Cfg.Profile.IsProduction() {
				attrs = append(attrs, "stack", string(panicStack))
			}
			slog.ErrorContext(c.Context(), "secure command panic recovered", attrs...)
			renderSecureError(c, fiber.ErrInternalServerError)
		case downstreamErr != nil:
			commandtrace.SetErrorType(c.Context(), fmt.Sprintf("%T", downstreamErr))
			renderSecureError(c, downstreamErr)
		}

		// 5. Encrypt Response Body (NK Message 2: <- e, ee, payload)
		finishEncrypt := commandtrace.MeasurePhase(c.Context(), "noise_encrypt")
		responseBody := c.Response().Body()
		encrypted, err := nc.EncryptPacket(responseBody)
		if err != nil {
			finishEncrypt()
			commandtrace.SetErrorType(c.Context(), fmt.Sprintf("%T", err))
			slog.WarnContext(c.Context(), "noise response encryption failed", "error_type", fmt.Sprintf("%T", err))
			finishResponse := commandtrace.MeasurePhase(c.Context(), "response_encode")
			defer finishResponse()
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		c.Response().SetBody(encrypted)
		c.Response().Header.Set("Content-Type", "application/octet-stream")
		finishEncrypt()

		return nil
	}
}

func nextSecureHandler(c fiber.Ctx) (recovered any, panicStack []byte, err error) {
	defer func() {
		if recovered = recover(); recovered != nil {
			panicStack = debug.Stack()
		}
	}()
	err = c.Next()
	return nil, nil, err
}

func renderSecureError(c fiber.Ctx, cause error) {
	finishResponse := commandtrace.MeasurePhase(c.Context(), "response_encode")
	defer finishResponse()
	if handlerErr := c.App().ErrorHandler(c, cause); handlerErr != nil {
		commandtrace.SetErrorType(c.Context(), fmt.Sprintf("%T", handlerErr))
		slog.ErrorContext(c.Context(), "secure command error handler failed", "error_type", fmt.Sprintf("%T", handlerErr))
		c.Response().SetStatusCode(fiber.StatusInternalServerError)
		c.Response().Header.SetContentType("text/plain; charset=utf-8")
		c.Response().SetBodyString("Internal Server Error")
	}
}
