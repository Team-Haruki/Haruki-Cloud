package secure

import (
	"log"

	"haruki-cloud/internal/core/crypto"

	"github.com/gofiber/fiber/v3"
)

// Config defines the config for Secure middleware.
type Config struct {
	// ServerPrivateKey is the server's private key used for Noise IK.
	ServerPrivateKey *crypto.KeyPair
}

// New creates a new Secure middleware with Noise IK support.
func New(config Config) fiber.Handler {
	if config.ServerPrivateKey == nil {
		log.Fatal("Secure middleware: ServerPrivateKey is required")
	}

	return func(c fiber.Ctx) error {
		// 1. Initialize Noise Responder (IK Pattern)
		// Responder doesn't need peer static key initially for IK.
		nc, err := crypto.NewHandshake(config.ServerPrivateKey, nil, false)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Crypto init failed"})
		}

		// 2. Read Request Body (Handshake Message 1)
		// For IK, the first message contains Client's Static Key (encrypted) and Payload (Encrypted).
		ciphertext := c.Body()
		if len(ciphertext) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Empty body"})
		}

		plaintext, err := nc.DecryptPacket(ciphertext)
		if err != nil {
			log.Printf("SecureMiddleware: Handshake/Decrypt failed: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Secure handshake failed (Decrypt)"})
		}

		// 3. Verify Client Identity (Optional but recommended)
		peerStatic := nc.GetPeerStatic()
		if peerStatic != nil {
			// TODO: Verify peerStatic against a whitelist DB
			// log.Printf("SecureMiddleware: Client Identity: %x", peerStatic)
		}

		// 4. Set Request Body to Plaintext
		c.Request().SetBody(plaintext)
		// Ensure Content-Type is msgpack for binding
		c.Request().Header.Set("Content-Type", "application/msgpack")

		// 5. Continue stack
		if err := c.Next(); err != nil {
			return err
		}

		// 6. Encrypt Response Body (Handshake Message 2)
		// For IK, the second message (Response) finishes the handshake.
		responseBody := c.Response().Body()

		// Even if body is empty (e.g. 204), Noise protocol expects the flow to complete if we want strictness.
		// But `EncryptPacket` handles writing the next message step even with empty payload.
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
