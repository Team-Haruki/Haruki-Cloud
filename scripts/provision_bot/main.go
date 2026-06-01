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

func main() {
	var (
		configPath = flag.String("config", "haruki-cloud.yaml", "path to haruki-cloud.yaml")
		qq         = flag.Int64("qq", 0, "QQ号 (required)")
		explicitID = flag.Int("bot-id", 0, "explicit 8-digit bot_id to assign (random if omitted)")
		force      = flag.Bool("force", false, "reset credential if user already exists")
		rebind     = flag.Bool("rebind", false, "change bot_id of existing owner (requires --bot-id); cascades to related records")
	)
	flag.Parse()

	if *qq == 0 {
		fmt.Fprintln(os.Stderr, "error: --qq is required")
		flag.Usage()
		os.Exit(1)
	}
	if *rebind && *explicitID == 0 {
		fatalf("--rebind requires --bot-id to specify the new bot_id")
	}
	if *explicitID != 0 && (*explicitID < 10_000_000 || *explicitID > 99_999_999) {
		fatalf("--bot-id must be an 8-digit integer (10000000–99999999)")
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

	if existing != nil && !*force && !*rebind {
		fatalf("QQ %d is already registered (bot_id=%d). Use --force to reset credential or --rebind --bot-id <id> to change bot_id.", *qq, existing.BotID)
	}
	if existing == nil && *rebind {
		fatalf("QQ %d is not registered; --rebind requires an existing user", *qq)
	}

	// -- rebind path: change bot_id for an existing owner and cascade --
	if *rebind {
		newBotID := *explicitID
		oldBotID := existing.BotID

		// Ensure the new bot_id is not already taken by someone else.
		taken, err := client.User.Query().Where(user.BotIDEQ(newBotID)).Exist(ctx)
		if err != nil {
			fatalf("check bot_id conflict: %v", err)
		}
		if taken {
			fatalf("bot_id %d is already assigned to another user", newBotID)
		}

		upd := client.User.UpdateOneID(existing.ID).SetBotID(newBotID)
		if *force {
			plainCredential, err := generateCredential()
			if err != nil {
				fatalf("generate credential: %v", err)
			}
			hashedCredential, err := hashCredential(plainCredential)
			if err != nil {
				fatalf("hash credential: %v", err)
			}
			upd = upd.SetCredential(hashedCredential)
			_, err = upd.Save(ctx)
			if err != nil {
				fatalf("update user: %v", err)
			}
			// Cascade bot_id update to requests_ranking.
			affected, err := client.RequestsRanking.Update().
				Where(requestsranking.BotIDEQ(oldBotID)).
				SetBotID(newBotID).
				Save(ctx)
			if err != nil {
				fatalf("cascade update requests_ranking: %v", err)
			}
			token, err := signJWT(newBotID, plainCredential, cfg.HarukiBotDB.CredentialSignToken)
			if err != nil {
				fatalf("sign JWT: %v", err)
			}
			fmt.Printf("(bot_id changed %d → %d, credential reset, %d ranking rows updated)\n", oldBotID, newBotID, affected)
			fmt.Printf("QQ号:       %d\n", *qq)
			fmt.Printf("Bot ID:     %d\n", newBotID)
			fmt.Printf("Credential: %s\n", token)
		} else {
			_, err = upd.Save(ctx)
			if err != nil {
				fatalf("update user: %v", err)
			}
			affected, err := client.RequestsRanking.Update().
				Where(requestsranking.BotIDEQ(oldBotID)).
				SetBotID(newBotID).
				Save(ctx)
			if err != nil {
				fatalf("cascade update requests_ranking: %v", err)
			}
			fmt.Printf("(bot_id changed %d → %d, credential unchanged, %d ranking rows updated)\n", oldBotID, newBotID, affected)
			fmt.Printf("QQ号:       %d\n", *qq)
			fmt.Printf("Bot ID:     %d\n", newBotID)
		}
		return
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
		// New user: use explicit bot_id or generate a unique 8-digit one.
		if *explicitID != 0 {
			taken, err := client.User.Query().Where(user.BotIDEQ(*explicitID)).Exist(ctx)
			if err != nil {
				fatalf("check bot_id conflict: %v", err)
			}
			if taken {
				fatalf("bot_id %d is already assigned to another user", *explicitID)
			}
			botID = *explicitID
		} else {
			botID, err = generateUniqueBotID(ctx, client)
			if err != nil {
				fatalf("generate bot_id: %v", err)
			}
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

	token, err := signJWT(botID, plainCredential, cfg.HarukiBotDB.CredentialSignToken)
	if err != nil {
		fatalf("sign JWT: %v", err)
	}

	fmt.Printf("QQ号:       %d\n", *qq)
	fmt.Printf("Bot ID:     %d\n", botID)
	fmt.Printf("Credential: %s\n", token)
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
