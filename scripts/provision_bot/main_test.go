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
	createProvisionTestDatabase(t, dsn)

	configPath := filepath.Join(t.TempDir(), "haruki-cloud.yaml")
	configBody := fmt.Sprintf("profile: dev\nharuki_bot:\n  db_type: sqlite3\n  db_url: %q\n  credential_sign_token: provision-test-secret\n", dsn)
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	runProvisionMain(t, "--config", configPath, "--qq", "10001")
	existing := queryProvisionTestUser(t, dsn, 10001)
	if existing.BotID < 10_000_000 || existing.BotID > 99_999_999 {
		t.Fatalf("generated bot id = %d", existing.BotID)
	}
	firstHash := existing.Credential

	runProvisionMain(t, "--config", configPath, "--qq", "10001", "--force")
	existing = queryProvisionTestUser(t, dsn, 10001)
	if existing.Credential == firstHash {
		t.Fatalf("credential was not reset: %#v", existing)
	}

	runProvisionMain(t, "--config", configPath, "--qq", "10001", "--rebind", "--bot-id", "23456789")
	existing = queryProvisionTestUser(t, dsn, 10001)
	if existing.BotID != 23456789 {
		t.Fatalf("rebound user = %#v", existing)
	}
	secondHash := existing.Credential

	runProvisionMain(t, "--config", configPath, "--qq", "10001", "--rebind", "--bot-id", "34567890", "--force")
	existing = queryProvisionTestUser(t, dsn, 10001)
	if existing.BotID != 34567890 || existing.Credential == secondHash {
		t.Fatalf("forced rebound user = %#v", existing)
	}
}

func createProvisionTestDatabase(t *testing.T, dsn string) {
	t.Helper()
	client, err := botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bot database: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create bot schema: %v", err)
	}
}

func queryProvisionTestUser(t *testing.T, dsn string, qq int64) *botDB.User {
	t.Helper()
	client, err := botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bot database: %v", err)
	}
	defer client.Close()
	existing, err := client.User.Query().Where(user.OwnerUserIDEQ(qq)).Only(context.Background())
	if err != nil {
		t.Fatalf("query provisioned user: %v", err)
	}
	return existing
}

func TestValidateProvisionOptions(t *testing.T) {
	for _, test := range []struct {
		name    string
		options provisionOptions
		wantErr bool
	}{
		{name: "default"},
		{name: "rebind missing id", options: provisionOptions{rebind: true}, wantErr: true},
		{name: "id too small", options: provisionOptions{explicitID: 9_999_999}, wantErr: true},
		{name: "id too large", options: provisionOptions{explicitID: 100_000_000}, wantErr: true},
		{name: "valid id", options: provisionOptions{explicitID: 12_345_678}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateProvisionOptions(test.options); (err != nil) != test.wantErr {
				t.Fatalf("validateProvisionOptions() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestProvisionBotRejectsDuplicateAndInvalidRebind(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?_fk=1", filepath.Join(t.TempDir(), "bot-errors.db"))
	createProvisionTestDatabase(t, dsn)
	client, err := botDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open bot database: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	options := provisionOptions{qq: 10001, explicitID: 12_345_678}
	if err := provisionBot(ctx, client, "secret", options); err != nil {
		t.Fatalf("initial provision: %v", err)
	}
	if err := provisionBot(ctx, client, "secret", options); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate provision error = %v", err)
	}
	if err := provisionBot(ctx, client, "secret", provisionOptions{qq: 20002, explicitID: 23_456_789, rebind: true}); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("missing rebind error = %v", err)
	}
	if _, err := selectProvisionedBotID(ctx, client, 12_345_678); err == nil || !strings.Contains(err.Error(), "already assigned") {
		t.Fatalf("bot ID conflict error = %v", err)
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
