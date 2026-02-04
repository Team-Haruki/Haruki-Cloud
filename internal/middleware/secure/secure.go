package secure

import (
	"encoding/base64"
	"log"

	"haruki-cloud/internal/core/crypto"

	"github.com/gofiber/fiber/v3"
)

// Config defines the config for Secure middleware.
type Config struct {
	// ServerPrivateKey is the server's private key used for ECDH.
	ServerPrivateKey *crypto.KeyPair
}

// New creates a new Secure middleware.
func New(config Config) fiber.Handler {
	if config.ServerPrivateKey == nil {
		log.Fatal("Secure middleware: ServerPrivateKey is required")
	}

	return func(c fiber.Ctx) error {
		// 1. Read X-Client-Pub-Key header
		clientPubBase64 := c.Get("X-Client-Pub-Key")
		if clientPubBase64 == "" {
			// If no key provided, return error (Enforce encryption)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Missing X-Client-Pub-Key header",
			})
		}

		clientPubBytes, err := base64.StdEncoding.DecodeString(clientPubBase64)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid X-Client-Pub-Key header",
			})
		}

		// 2. Derive Shared Secret
		sharedSecret, err := crypto.DeriveSharedSecret(config.ServerPrivateKey.PrivateKey, clientPubBytes)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Key exchange failed",
			})
		}

		// 3. Decrypt Request Body (if present)
		if len(c.Body()) > 0 {
			decrypted, err := crypto.DecryptAESGCM(sharedSecret, c.Body())
			if err != nil {
				log.Printf("SecureMiddleware: Decryption failed: %v", err)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Decryption failed",
				})
			}
			log.Printf("SecureMiddleware: Decrypted body (%d bytes): %x", len(decrypted), decrypted)
			c.Request().SetBody(decrypted)
		} else {
			log.Println("SecureMiddleware: No body to decrypt")
		}

		// Ensure Content-Type is msgpack for binding
		c.Request().Header.Set("Content-Type", "application/msgpack")

		// 4. Continue stack
		if err := c.Next(); err != nil {
			return err
		}

		// 5. Encrypt Response Body
		responseBody := c.Response().Body()
		if len(responseBody) > 0 {
			encrypted, err := crypto.EncryptAESGCM(sharedSecret, responseBody)
			if err != nil {
				log.Printf("Encryption failed: %v", err)
				return c.SendStatus(fiber.StatusInternalServerError)
			}
			c.Response().SetBody(encrypted)
			// Set generic content type
			c.Response().Header.Set("Content-Type", "application/octet-stream")
		}

		return nil
	}
}
