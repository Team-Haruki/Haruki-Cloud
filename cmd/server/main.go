package main

import (
	"log"

	"haruki-cloud/internal/core/crypto"
	// "haruki-cloud/internal/middleware/secure"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

func main() {
	// 1. Initialize Crypto
	serverKeys, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Failed to generate server keys: %v", err)
	}
	_ = serverKeys

	// 2. Setup Fiber
	app := fiber.New()

	app.Use(logger.New())

	// Secure Group
	// api := app.Group("/api", secure.New(secure.Config{
	// 	ServerPrivateKey: serverKeys,
	// }))

	// TODO: Register your business routes here
	// e.g. api.Post("/drawing", handler.HandleDrawing)

	log.Println("Server starting on :3000")
	log.Fatal(app.Listen(":3000"))
}
