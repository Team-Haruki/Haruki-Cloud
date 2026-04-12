package secure

import (
	"log"

	"haruki-cloud/internal/core/crypto"

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
		log.Fatal("Secure middleware: ServerPrivateKey is required")
	}

	return func(c fiber.Ctx) error {
		// 1. Initialize Noise NK Responder
		nc, err := crypto.NewResponder(config.ServerPrivateKey)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Crypto init failed"})
		}

		// 2. Read Request Body (NK Message 1: -> e, es, payload)
		ciphertext := c.Body()
		if len(ciphertext) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Empty body"})
		}

		plaintext, err := nc.DecryptPacket(ciphertext)
		if err != nil {
			log.Printf("SecureMiddleware: Handshake/Decrypt failed: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Secure handshake failed (Decrypt)"})
		}

		// 3. Set Request Body to Plaintext
		c.Request().SetBody(plaintext)
		c.Request().Header.Set("Content-Type", "application/msgpack")
		c.Locals("secure_noise", true)

		// 4. Continue stack
		if err := c.Next(); err != nil {
			return err
		}

		// 5. Encrypt Response Body (NK Message 2: <- e, ee, payload)
		responseBody := c.Response().Body()
		encrypted, err := nc.EncryptPacket(responseBody)
		if err != nil {
			log.Printf("SecureMiddleware: Encryption failed: %v", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		c.Response().SetBody(encrypted)
		c.Response().Header.Set("Content-Type", "application/octet-stream")

		return nil
	}
}
