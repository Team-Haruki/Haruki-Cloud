// provision_bot is a CLI tool for manually provisioning a bot user.
//
// It generates a unique bot_id and credential, writes the user record to the
// database configured in haruki-db-configs.yaml, and prints the signed JWT
// credential ready to paste into the client's configs.yaml.
//
// Usage:
//
//	go run ./scripts/provision_bot --qq <QQ号> [--config haruki-db-configs.yaml] [--force]
//
// Flags:
//
//	--qq      QQ号 (required) — owner QQ number of the bot
//	--config  path to haruki-db-configs.yaml (default: haruki-db-configs.yaml)
//	--force   reset credential if the user already exists
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"math/big"
	"os"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/user"
	harukiConfig "haruki-cloud/config"
)

func main() {
	var (
		configPath = flag.String("config", "haruki-db-configs.yaml", "path to haruki-db-configs.yaml")
		qq         = flag.Int64("qq", 0, "QQ号 (required)")
		force      = flag.Bool("force", false, "reset credential if user already exists")
	)
	flag.Parse()

	if *qq == 0 {
		fmt.Fprintln(os.Stderr, "error: --qq is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := harukiConfig.ReadConfig(*configPath)
	if err != nil {
		fatalf("read config: %v", err)
	}
	if cfg.HarukiBotDB.DBType == "" || cfg.HarukiBotDB.DBURL == "" {
		fatalf("haruki_bot_db not configured in %s", *configPath)
	}
	if cfg.HarukiBotDB.CredentialSignToken == "" {
		fatalf("credential_sign_token not configured in %s", *configPath)
	}

	client, err := botDB.Open(cfg.HarukiBotDB.DBType, cfg.HarukiBotDB.DBURL)
	if err != nil {
		fatalf("open bot DB: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Check if the QQ number is already registered.
	existing, err := client.User.Query().Where(user.OwnerUserIDEQ(*qq)).First(ctx)
	if err != nil && !botDB.IsNotFound(err) {
		fatalf("query user: %v", err)
	}

	if existing != nil && !*force {
		fatalf("QQ %d is already registered (bot_id=%d). Use --force to reset credential.", *qq, existing.BotID)
	}

	// Generate credential (plaintext, 32 random bytes → base64url).
	plainCredential, err := generateCredential()
	if err != nil {
		fatalf("generate credential: %v", err)
	}

	// bcrypt-hash for storage.
	hashedCredential, err := hashCredential(plainCredential)
	if err != nil {
		fatalf("hash credential: %v", err)
	}

	var botID int
	if existing != nil {
		// --force: update credential, keep the existing bot_id.
		botID = existing.BotID
		_, err = client.User.UpdateOneID(existing.ID).
			SetCredential(hashedCredential).
			Save(ctx)
		if err != nil {
			fatalf("update user: %v", err)
		}
		fmt.Printf("(existing user — credential reset)\n")
	} else {
		// New user: generate a unique 8-digit bot_id.
		botID, err = generateUniqueBotID(ctx, client)
		if err != nil {
			fatalf("generate bot_id: %v", err)
		}
		_, err = client.User.
			Create().
			SetOwnerUserID(*qq).
			SetBotID(botID).
			SetCredential(hashedCredential).
			Save(ctx)
		if err != nil {
			fatalf("create user: %v", err)
		}
	}

	// Sign JWT — payload matches what the auth handler expects.
	payload := jwt.MapClaims{
		"bot_id":     fmt.Sprintf("%d", botID),
		"credential": plainCredential,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).
		SignedString([]byte(cfg.HarukiBotDB.CredentialSignToken))
	if err != nil {
		fatalf("sign JWT: %v", err)
	}

	fmt.Printf("QQ号:       %d\n", *qq)
	fmt.Printf("Bot ID:     %d\n", botID)
	fmt.Printf("Credential: %s\n", token)
}

// generateUniqueBotID returns a random 8-digit bot_id not already in use.
func generateUniqueBotID(ctx context.Context, client *botDB.Client) (int, error) {
	for i := 0; i < 20; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(90_000_000))
		if err != nil {
			return 0, err
		}
		id := int(n.Int64() + 10_000_000)
		exists, err := client.User.Query().Where(user.BotIDEQ(id)).Exist(ctx)
		if err != nil {
			return 0, err
		}
		if !exists {
			return id, nil
		}
	}
	return 0, fmt.Errorf("could not find a unique bot_id after 20 attempts")
}

func generateCredential() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func hashCredential(credential string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(credential), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
