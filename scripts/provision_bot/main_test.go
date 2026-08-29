package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/user"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func runProvisionMain(t *testing.T, args ...string) {
	t.Helper()
	originalArgs := os.Args
	originalFlags := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("provision_bot_test", flag.ContinueOnError)
	os.Args = append([]string{"provision_bot"}, args...)
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlags
	})
	main()
	flag.CommandLine = originalFlags
	os.Args = originalArgs
}

func TestProvisionBotSuccessfulLifecycle(t *testing.T) {
	for _, name := range []string{"HARUKI_BOT_DB_TYPE", "HARUKI_BOT_DB_URL", "HARUKI_BOT_CREDENTIAL_SIGN_TOKEN"} {
		t.Setenv(name, "")
	}
	dbPath := filepath.Join(t.TempDir(), "bot.db")
	dsn := fmt.Sprintf("file:%s?_fk=1", dbPath)
	client, err := botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bot database: %v", err)
	}
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create bot schema: %v", err)
	}
	_ = client.Close()

	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	configBody := fmt.Sprintf("profile: dev\nharuki_bot:\n  db_type: sqlite3\n  db_url: %q\n  credential_sign_token: provision-test-secret\n", dsn)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	runProvisionMain(t, "--config", configPath, "--qq", "10001")
	client, err = botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("reopen bot database: %v", err)
	}
	existing, err := client.User.Query().Where(user.OwnerUserIDEQ(10001)).Only(context.Background())
	if err != nil {
		t.Fatalf("query provisioned user: %v", err)
	}
	if existing.BotID < 10_000_000 || existing.BotID > 99_999_999 {
		t.Fatalf("generated bot id = %d", existing.BotID)
	}
	firstHash := existing.Credential
	_ = client.Close()

	runProvisionMain(t, "--config", configPath, "--qq", "10001", "--force")
	client, err = botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("reopen after force: %v", err)
	}
	existing, err = client.User.Query().Where(user.OwnerUserIDEQ(10001)).Only(context.Background())
	if err != nil || existing.Credential == firstHash {
		t.Fatalf("credential reset result = %#v, %v", existing, err)
	}
	_ = client.Close()

	runProvisionMain(t, "--config", configPath, "--qq", "10001", "--rebind", "--bot-id", "23456789")
	client, err = botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("reopen after rebind: %v", err)
	}
	existing, err = client.User.Query().Where(user.OwnerUserIDEQ(10001)).Only(context.Background())
	if err != nil || existing.BotID != 23456789 {
		t.Fatalf("rebound user = %#v, %v", existing, err)
	}
	secondHash := existing.Credential
	_ = client.Close()

	runProvisionMain(t, "--config", configPath, "--qq", "10001", "--rebind", "--bot-id", "34567890", "--force")
	client, err = botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("reopen after forced rebind: %v", err)
	}
	defer client.Close()
	existing, err = client.User.Query().Where(user.OwnerUserIDEQ(10001)).Only(context.Background())
	if err != nil || existing.BotID != 34567890 || existing.Credential == secondHash {
		t.Fatalf("forced rebound user = %#v, %v", existing, err)
	}
}

func TestProvisionCredentialHelpers(t *testing.T) {
	credential, err := generateCredential()
	if err != nil || credential == "" {
		t.Fatalf("generate credential = %q, %v", credential, err)
	}
	hash, err := hashCredential(credential)
	if err != nil {
		t.Fatalf("hash credential: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(credential)); err != nil {
		t.Fatalf("credential hash mismatch: %v", err)
	}

	signed, err := signJWT(12345678, credential, "signing-secret")
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	parsed, err := jwt.Parse(signed, func(token *jwt.Token) (any, error) {
		return []byte("signing-secret"), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		t.Fatalf("parse signed JWT: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || claims["bot_id"] != "12345678" || !strings.EqualFold(fmt.Sprint(claims["credential"]), credential) {
		t.Fatalf("JWT claims = %#v", parsed.Claims)
	}
}
