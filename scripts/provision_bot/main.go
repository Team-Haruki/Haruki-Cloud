// provision_bot is a CLI tool for manually provisioning a bot user.
//
// It generates a unique bot_id and credential, writes the user record to the
// database configured in haruki-cloud.yaml, and prints the signed JWT
// credential ready to paste into the client's configs.yaml.
//
// Usage:
//
//	go run ./scripts/provision_bot --qq <QQ号> [--config haruki-cloud.yaml] [--bot-id <id>] [--force] [--rebind]
//
// Flags:
//
//	--qq       QQ号 (required) — owner QQ number of the bot
//	--config   path to haruki-cloud.yaml (default: haruki-cloud.yaml)
//	--bot-id   explicit bot_id to assign (8-digit int); random if omitted
//	--force    reset credential if the user already exists
//	--rebind   change bot_id of an existing owner to the value given by --bot-id,
//	           cascading the update to all associated records (e.g. requests_ranking)
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

	harukiConfig "haruki-cloud/config"
	botDB "haruki-cloud/database/bot"
	"haruki-cloud/database/bot/requestsranking"
	"haruki-cloud/database/bot/user"
)

type provisionOptions struct {
	configPath string
	qq         int64
	explicitID int
	force      bool
	rebind     bool
}

func main() {
	options := parseProvisionOptions()
	if options.qq == 0 {
		fmt.Fprintln(os.Stderr, "error: --qq is required")
		flag.Usage()
		os.Exit(1)
	}
	if err := validateProvisionOptions(options); err != nil {
		fatalf("%v", err)
	}

	cfg, err := harukiConfig.ReadConfig(options.configPath)
	if err != nil {
		fatalf("read config: %v", err)
	}
	if cfg.HarukiBotDB.DBType == "" || cfg.HarukiBotDB.DBURL == "" {
		fatalf("haruki_bot_db not configured in %s", options.configPath)
	}
	if cfg.HarukiBotDB.CredentialSignToken == "" {
		fatalf("credential_sign_token not configured in %s", options.configPath)
	}

	client, err := botDB.Open(cfg.HarukiBotDB.DBType, cfg.HarukiBotDB.DBURL)
	if err != nil {
		fatalf("open bot DB: %v", err)
	}
	defer client.Close()

	if err := provisionBot(context.Background(), client, cfg.HarukiBotDB.CredentialSignToken, options); err != nil {
		fatalf("%v", err)
	}
}

func parseProvisionOptions() provisionOptions {
	configPath := flag.String("config", "haruki-cloud.yaml", "path to haruki-cloud.yaml")
	qq := flag.Int64("qq", 0, "QQ号 (required)")
	explicitID := flag.Int("bot-id", 0, "explicit 8-digit bot_id to assign (random if omitted)")
	force := flag.Bool("force", false, "reset credential if user already exists")
	rebind := flag.Bool("rebind", false, "change bot_id of existing owner (requires --bot-id); cascades to related records")
	flag.Parse()
	return provisionOptions{configPath: *configPath, qq: *qq, explicitID: *explicitID, force: *force, rebind: *rebind}
}

func validateProvisionOptions(options provisionOptions) error {
	if options.rebind && options.explicitID == 0 {
		return fmt.Errorf("--rebind requires --bot-id to specify the new bot_id")
	}
	if options.explicitID != 0 && (options.explicitID < 10_000_000 || options.explicitID > 99_999_999) {
		return fmt.Errorf("--bot-id must be an 8-digit integer (10000000–99999999)")
	}
	return nil
}

func provisionBot(ctx context.Context, client *botDB.Client, signingToken string, options provisionOptions) error {
	existing, err := client.User.Query().Where(user.OwnerUserIDEQ(options.qq)).First(ctx)
	if err != nil && !botDB.IsNotFound(err) {
		return fmt.Errorf("query user: %w", err)
	}
	if existing != nil && !options.force && !options.rebind {
		return fmt.Errorf("QQ %d is already registered (bot_id=%d). Use --force to reset credential or --rebind --bot-id <id> to change bot_id", options.qq, existing.BotID)
	}
	if existing == nil && options.rebind {
		return fmt.Errorf("QQ %d is not registered; --rebind requires an existing user", options.qq)
	}
	if options.rebind {
		return rebindProvisionedBot(ctx, client, existing, signingToken, options)
	}
	return createOrResetProvisionedBot(ctx, client, existing, signingToken, options)
}

func rebindProvisionedBot(ctx context.Context, client *botDB.Client, existing *botDB.User, signingToken string, options provisionOptions) error {
	newBotID := options.explicitID
	oldBotID := existing.BotID
	taken, err := client.User.Query().Where(user.BotIDEQ(newBotID)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check bot_id conflict: %w", err)
	}
	if taken {
		return fmt.Errorf("bot_id %d is already assigned to another user", newBotID)
	}
	if options.force {
		return rebindWithCredentialReset(ctx, client, existing, signingToken, options, oldBotID, newBotID)
	}
	if _, err := client.User.UpdateOneID(existing.ID).SetBotID(newBotID).Save(ctx); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	affected, err := cascadeRankingBotID(ctx, client, oldBotID, newBotID)
	if err != nil {
		return err
	}
	fmt.Printf("(bot_id changed %d → %d, credential unchanged, %d ranking rows updated)\n", oldBotID, newBotID, affected)
	fmt.Printf("QQ号:       %d\n", options.qq)
	fmt.Printf("Bot ID:     %d\n", newBotID)
	return nil
}

func rebindWithCredentialReset(ctx context.Context, client *botDB.Client, existing *botDB.User, signingToken string, options provisionOptions, oldBotID int, newBotID int) error {
	plainCredential, err := generateCredential()
	if err != nil {
		return fmt.Errorf("generate credential: %w", err)
	}
	hashedCredential, err := hashCredential(plainCredential)
	if err != nil {
		return fmt.Errorf("hash credential: %w", err)
	}
	if _, err := client.User.UpdateOneID(existing.ID).SetBotID(newBotID).SetCredential(hashedCredential).Save(ctx); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	affected, err := cascadeRankingBotID(ctx, client, oldBotID, newBotID)
	if err != nil {
		return err
	}
	token, err := signJWT(newBotID, plainCredential, signingToken)
	if err != nil {
		return fmt.Errorf("sign JWT: %w", err)
	}
	fmt.Printf("(bot_id changed %d → %d, credential reset, %d ranking rows updated)\n", oldBotID, newBotID, affected)
	fmt.Printf("QQ号:       %d\n", options.qq)
	fmt.Printf("Bot ID:     %d\n", newBotID)
	fmt.Printf("Credential: %s\n", token)
	return nil
}

func createOrResetProvisionedBot(ctx context.Context, client *botDB.Client, existing *botDB.User, signingToken string, options provisionOptions) error {
	plainCredential, err := generateCredential()
	if err != nil {
		return fmt.Errorf("generate credential: %w", err)
	}
	hashedCredential, err := hashCredential(plainCredential)
	if err != nil {
		return fmt.Errorf("hash credential: %w", err)
	}
	botID, err := saveProvisionedBot(ctx, client, existing, hashedCredential, options)
	if err != nil {
		return err
	}
	token, err := signJWT(botID, plainCredential, signingToken)
	if err != nil {
		return fmt.Errorf("sign JWT: %w", err)
	}
	fmt.Printf("QQ号:       %d\n", options.qq)
	fmt.Printf("Bot ID:     %d\n", botID)
	fmt.Printf("Credential: %s\n", token)
	return nil
}

func saveProvisionedBot(ctx context.Context, client *botDB.Client, existing *botDB.User, hashedCredential string, options provisionOptions) (int, error) {
	if existing != nil {
		if _, err := client.User.UpdateOneID(existing.ID).SetCredential(hashedCredential).Save(ctx); err != nil {
			return 0, fmt.Errorf("update user: %w", err)
		}
		fmt.Printf("(existing user — credential reset)\n")
		return existing.BotID, nil
	}
	botID, err := selectProvisionedBotID(ctx, client, options.explicitID)
	if err != nil {
		return 0, err
	}
	if _, err := client.User.Create().SetOwnerUserID(options.qq).SetBotID(botID).SetCredential(hashedCredential).Save(ctx); err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	return botID, nil
}

func selectProvisionedBotID(ctx context.Context, client *botDB.Client, explicitID int) (int, error) {
	if explicitID == 0 {
		botID, err := generateUniqueBotID(ctx, client)
		if err != nil {
			return 0, fmt.Errorf("generate bot_id: %w", err)
		}
		return botID, nil
	}
	taken, err := client.User.Query().Where(user.BotIDEQ(explicitID)).Exist(ctx)
	if err != nil {
		return 0, fmt.Errorf("check bot_id conflict: %w", err)
	}
	if taken {
		return 0, fmt.Errorf("bot_id %d is already assigned to another user", explicitID)
	}
	return explicitID, nil
}

func cascadeRankingBotID(ctx context.Context, client *botDB.Client, oldBotID int, newBotID int) (int, error) {
	affected, err := client.RequestsRanking.Update().
		Where(requestsranking.BotIDEQ(oldBotID)).
		SetBotID(newBotID).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("cascade update requests_ranking: %w", err)
	}
	return affected, nil
}

func signJWT(botID int, plainCredential, signingToken string) (string, error) {
	payload := jwt.MapClaims{
		"bot_id":     fmt.Sprintf("%d", botID),
		"credential": plainCredential,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, payload).
		SignedString([]byte(signingToken))
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
