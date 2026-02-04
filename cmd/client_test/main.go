package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"

	"haruki-cloud/internal/core/crypto"

	"github.com/vmihailenco/msgpack/v5"
)

type TestRequest struct {
	Name string `json:"name" msgpack:"name"`
	Age  int    `json:"age" msgpack:"age"`
}

type TestResponse struct {
	Message string `json:"message" msgpack:"message"`
	Success bool   `json:"success" msgpack:"success"`
}

func main() {
	baseURL := "http://localhost:3000"

	// 1. Get Server Public Key
	resp, err := http.Get(baseURL + "/key")
	if err != nil {
		log.Fatalf("Failed to get server key: %v", err)
	}
	defer resp.Body.Close()
	serverPubBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read server key: %v", err)
	}
	fmt.Printf("Got Server Public Key: %d bytes\n", len(serverPubBytes))

	// 2. Generate Client Keys
	clientKeys, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Failed to generate client keys: %v", err)
	}

	// 3. Derive Shared Secret
	sharedSecret, err := crypto.DeriveSharedSecret(clientKeys.PrivateKey, serverPubBytes)
	if err != nil {
		log.Fatalf("Failed to derive shared secret: %v", err)
	}
	fmt.Printf("Derived Shared Secret: %x\n", sharedSecret)

	// 4. Prepare Request
	payload := TestRequest{
		Name: "Antigravity",
		Age:  1,
	}
	payloadBytes, err := msgpack.Marshal(payload)
	if err != nil {
		log.Fatalf("Marshal failed: %v", err)
	}

	// 5. Encrypt Request
	encryptedReq, err := crypto.EncryptAESGCM(sharedSecret, payloadBytes)
	if err != nil {
		log.Fatalf("Encrypt failed: %v", err)
	}

	// 6. Send Request
	req, _ := http.NewRequest("POST", baseURL+"/api/hello", bytes.NewReader(encryptedReq))
	req.Header.Set("Content-Type", "application/octet-stream") // Or whatever

	// Set Client Public Key Header
	clientPubBase64 := base64.StdEncoding.EncodeToString(clientKeys.PublicKey.Bytes())
	req.Header.Set("X-Client-Pub-Key", clientPubBase64)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer res.Body.Close()

	// 7. Decrypt Response (Always decrypt, even for errors, as middleware encrypts everything)
	encryptedRes, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Read response failed: %v", err)
	}

	decryptedRes, err := crypto.DecryptAESGCM(sharedSecret, encryptedRes)
	if err != nil {
		// If decryption fails, maybe it wasn't encrypted (e.g. 404 from Fiber default handler?)
		log.Printf("Decrypt response failed: %v. Raw Body: %s", err, string(encryptedRes))
		// Proceed to print raw body if decryption fails
		decryptedRes = encryptedRes
	}

	if res.StatusCode != 200 {
		log.Fatalf("Server Error: %d - %s", res.StatusCode, string(decryptedRes))
	}

	// 8. Unmarshal Response
	var responseData TestResponse
	err = msgpack.Unmarshal(decryptedRes, &responseData)
	if err != nil {
		log.Fatalf("Unmarshal response failed: %v", err)
	}

	fmt.Printf("Success! Server says: %s\n", responseData.Message)
}
