// cmd/importer imports legacy export data (exports/*.json) into the new database.
//
// Usage:
//
//	go run ./cmd/importer [flags]
//
// Flags:
//
//	--exports-dir   path to directory containing the JSON export files (default: ./exports)
//	--target        comma-separated list of targets: all, bindings, character-aliases,
//	                music-aliases, group-aliases (default: all)
//	--dry-run       parse and count records without writing anything
//
// DB connection is read from the same env vars / config file as the server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	json "github.com/bytedance/sonic"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	harukiConfig "haruki-cloud/config"
	pjskDB "haruki-cloud/database/pjsk"
	usersDB "haruki-cloud/database/users"
	"haruki-cloud/database/users/user"
	"haruki-cloud/internal/identity"

	"haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/gameaccount"
	"haruki-cloud/database/pjsk/groupalias"
	"haruki-cloud/database/pjsk/userbinding"

	_ "github.com/lib/pq"
)

// ──────────────────────────────────────────────
// JSON record types
// ──────────────────────────────────────────────

type bindRecord struct {
	ID        int    `json:"id"`
	IMUserID  string `json:"im_user_id"`
	UserID    string `json:"user_id"`
	IsPrivate int    `json:"is_private"`
}

type charAliasRecord struct {
	ID          int    `json:"id"`
	Alias       string `json:"alias"`
	CharacterID int    `json:"character_id"`
}

type musicAliasRecord struct {
	ID      int    `json:"id"`
	Alias   string `json:"alias"`
	MusicID int    `json:"music_id"`
}

type groupCharAliasRecord struct {
	ID          int    `json:"id"`
	GroupID     string `json:"group_id"`
	Alias       string `json:"alias"`
	CharacterID int    `json:"character_id"`
}

// ──────────────────────────────────────────────
// Platform detection
// ──────────────────────────────────────────────

// qqbotPattern matches "<sha256hex>_<appid>", e.g.
// 2e2344faaf7a342ee92d883caae14cf25e31345ff64db4bb1e05004ef8f901f6_102070411
var qqbotPattern = regexp.MustCompile(`^[0-9a-f]{64}_\d+$`)

func detectPlatform(imUserID string) (platform, userID string) {
	if qqbotPattern.MatchString(imUserID) {
		return "qqbot", imUserID
	}
	return "qq", imUserID
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func loadJSON[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []T
	if err := json.ConfigDefault.NewDecoder(f).Decode(&records); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return records, nil
}

func serverFromFilename(filename string) string {
	// "bind.json" → "jp", "cn_bind.json" → "cn"
	if filename == "bind.json" {
		return "jp"
	}
	return strings.TrimSuffix(filename, "_bind.json")
}

// ──────────────────────────────────────────────
// Import functions
// ──────────────────────────────────────────────

func importBindings(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, users *usersDB.Client, dryRun bool) error {
	files := []string{"bind.json", "cn_bind.json", "en_bind.json", "tw_bind.json", "kr_bind.json"}
	resolver := identity.NewResolver(users)

	total, inserted, skipped, failed := 0, 0, 0, 0

	for _, filename := range files {
		path := exportsDir + "/" + filename
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			log.Printf("[bindings] skipping %s: file not found", filename)
			continue
		}

		records, err := loadJSON[bindRecord](path)
		if err != nil {
			return fmt.Errorf("load %s: %w", filename, err)
		}

		server := serverFromFilename(filename)
		log.Printf("[bindings] %s → server=%s, %d records", filename, server, len(records))

		for i, rec := range records {
			total++
			if ctx.Err() != nil {
				return ctx.Err()
			}

			platform, platformUserID := detectPlatform(rec.IMUserID)
			visible := rec.IsPrivate == 0

			if dryRun {
				if (i+1)%5000 == 0 {
					log.Printf("[bindings/%s] dry-run progress: %d/%d", server, i+1, len(records))
				}
				inserted++
				continue
			}

			// 1. Resolve or create haruki user
			harukiUserID, err := resolver.ResolveOrCreate(ctx, platform, platformUserID)
			if err != nil {
				log.Printf("[bindings/%s] WARN record %d: resolve user (%s/%s): %v", server, rec.ID, platform, platformUserID, err)
				failed++
				continue
			}

			// 2. Upsert game account
			gameAcc, err := pjsk.GameAccount.Query().
				Where(gameaccount.ServerEQ(server), gameaccount.UserIDEQ(rec.UserID)).
				Only(ctx)
			if err != nil {
				if !pjskDB.IsNotFound(err) {
					log.Printf("[bindings/%s] WARN record %d: query game account: %v", server, rec.ID, err)
					failed++
					continue
				}
				gameAcc, err = pjsk.GameAccount.Create().
					SetServer(server).
					SetUserID(rec.UserID).
					Save(ctx)
				if err != nil {
					if pjskDB.IsConstraintError(err) {
						// Another row was inserted concurrently; re-query.
						gameAcc, err = pjsk.GameAccount.Query().
							Where(gameaccount.ServerEQ(server), gameaccount.UserIDEQ(rec.UserID)).
							Only(ctx)
					}
					if err != nil {
						log.Printf("[bindings/%s] WARN record %d: create game account: %v", server, rec.ID, err)
						failed++
						continue
					}
				}
			}

			// 3. Upsert user binding
			exists, err := pjsk.UserBinding.Query().
				Where(userbinding.HarukiUserID(harukiUserID), userbinding.GameAccountIDEQ(gameAcc.ID)).
				Exist(ctx)
			if err != nil {
				log.Printf("[bindings/%s] WARN record %d: query binding: %v", server, rec.ID, err)
				failed++
				continue
			}
			if exists {
				skipped++
				continue
			}

			displayOrder, err := nextDisplayOrder(ctx, pjsk, harukiUserID)
			if err != nil {
				log.Printf("[bindings/%s] WARN record %d: display order: %v", server, rec.ID, err)
				failed++
				continue
			}

			_, err = pjsk.UserBinding.Create().
				SetHarukiUserID(harukiUserID).
				SetGameAccountID(gameAcc.ID).
				SetDisplayOrder(displayOrder).
				SetVisible(visible).
				Save(ctx)
			if err != nil {
				if pjskDB.IsConstraintError(err) {
					skipped++
					continue
				}
				log.Printf("[bindings/%s] WARN record %d: create binding: %v", server, rec.ID, err)
				failed++
				continue
			}
			inserted++
		}

		log.Printf("[bindings/%s] done", server)
	}

	log.Printf("[bindings] total=%d inserted=%d skipped=%d failed=%d", total, inserted, skipped, failed)
	return nil
}

// nextDisplayOrder returns the next display_order value for a user's bindings.
func nextDisplayOrder(ctx context.Context, pjsk *pjskDB.Client, harukiUserID int) (int, error) {
	bindings, err := pjsk.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		All(ctx)
	if err != nil {
		return 0, err
	}
	return len(bindings), nil
}

// globalServerPriority defines the preference order when selecting a global default
// among users with multiple server bindings.
var globalServerPriority = []string{"jp", "cn", "tw", "en", "kr"}

// setDefaultBindings creates UserDefaultBinding rows for every user:
//   - 1 binding total   → global default ("default") + server default
//   - multiple bindings → global default by priority (jp>cn>tw>en>kr); per-server default
//
// Existing defaults are skipped (idempotent).
func setDefaultBindings(ctx context.Context, pjsk *pjskDB.Client, dryRun bool) error {
	if dryRun {
		log.Printf("[defaults] dry-run — skipping")
		return nil
	}
	// Load all bindings with their game accounts.
	allBindings, err := pjsk.UserBinding.Query().
		WithGameAccount().
		All(ctx)
	if err != nil {
		return fmt.Errorf("query bindings: %w", err)
	}

	type bindInfo struct {
		bindingID int
		server    string
	}
	// Group by haruki_user_id.
	byUser := make(map[int][]bindInfo, len(allBindings)/2)
	for _, b := range allBindings {
		if b.Edges.GameAccount == nil {
			continue
		}
		byUser[b.HarukiUserID] = append(byUser[b.HarukiUserID], bindInfo{
			bindingID: b.ID,
			server:    b.Edges.GameAccount.Server,
		})
	}

	log.Printf("[defaults] %d users to process", len(byUser))

	// Load existing defaults to avoid redundant writes.
	existingDefaults, err := pjsk.UserDefaultBinding.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("query existing defaults: %w", err)
	}
	type scopeKey struct {
		userID int
		server string
	}
	alreadySet := make(map[scopeKey]bool, len(existingDefaults))
	for _, d := range existingDefaults {
		alreadySet[scopeKey{d.HarukiUserID, d.Server}] = true
	}

	globalDefaultScope := "default"
	inserted, skipped, failed := 0, 0, 0

	createDefault := func(harukiUserID, bindingID int, scope string) {
		if alreadySet[scopeKey{harukiUserID, scope}] {
			skipped++
			return
		}
		_, err := pjsk.UserDefaultBinding.Create().
			SetHarukiUserID(harukiUserID).
			SetServer(scope).
			SetBindingID(bindingID).
			Save(ctx)
		if err != nil {
			if pjskDB.IsConstraintError(err) {
				skipped++
				return
			}
			log.Printf("[defaults] WARN user %d scope %s: %v", harukiUserID, scope, err)
			failed++
			return
		}
		alreadySet[scopeKey{harukiUserID, scope}] = true
		inserted++
	}

	for harukiUserID, binds := range byUser {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if len(binds) == 1 {
			// Single account: global default + server default.
			createDefault(harukiUserID, binds[0].bindingID, globalDefaultScope)
			createDefault(harukiUserID, binds[0].bindingID, binds[0].server)
			continue
		}

		// Multiple accounts: build per-server map (first encountered per server).
		byServer := make(map[string]int)
		for _, b := range binds {
			if _, exists := byServer[b.server]; !exists {
				byServer[b.server] = b.bindingID
			}
		}

		// Global default: first server in priority list that the user has.
		for _, srv := range globalServerPriority {
			if bindID, ok := byServer[srv]; ok {
				createDefault(harukiUserID, bindID, globalDefaultScope)
				break
			}
		}

		// Server-specific defaults: one per server.
		for srv, bindID := range byServer {
			createDefault(harukiUserID, bindID, srv)
		}
	}

	log.Printf("[defaults] inserted=%d skipped=%d failed=%d", inserted, skipped, failed)
	return nil
}

func importCharacterAliases(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, dryRun bool) error {
	records, err := loadJSON[charAliasRecord](exportsDir + "/character_alias.json")
	if err != nil {
		return err
	}
	log.Printf("[character-aliases] %d records", len(records))

	inserted, skipped, failed := 0, 0, 0
	for _, rec := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if dryRun {
			inserted++
			continue
		}
		exists, err := pjsk.Alias.Query().
			Where(
				alias.AliasType("character"),
				alias.AliasTypeID(rec.CharacterID),
				alias.AliasEQ(rec.Alias),
			).Exist(ctx)
		if err != nil {
			log.Printf("[character-aliases] WARN record %d: query: %v", rec.ID, err)
			failed++
			continue
		}
		if exists {
			skipped++
			continue
		}
		_, err = pjsk.Alias.Create().
			SetAliasType("character").
			SetAliasTypeID(rec.CharacterID).
			SetAlias(rec.Alias).
			Save(ctx)
		if err != nil {
			if pjskDB.IsConstraintError(err) {
				skipped++
				continue
			}
			log.Printf("[character-aliases] WARN record %d: insert: %v", rec.ID, err)
			failed++
			continue
		}
		inserted++
	}
	log.Printf("[character-aliases] inserted=%d skipped=%d failed=%d", inserted, skipped, failed)
	return nil
}

func importMusicAliases(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, dryRun bool) error {
	records, err := loadJSON[musicAliasRecord](exportsDir + "/music_alias.json")
	if err != nil {
		return err
	}
	log.Printf("[music-aliases] %d records", len(records))

	inserted, skipped, failed := 0, 0, 0
	for _, rec := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if dryRun {
			inserted++
			continue
		}
		exists, err := pjsk.Alias.Query().
			Where(
				alias.AliasType("music"),
				alias.AliasTypeID(rec.MusicID),
				alias.AliasEQ(rec.Alias),
			).Exist(ctx)
		if err != nil {
			log.Printf("[music-aliases] WARN record %d: query: %v", rec.ID, err)
			failed++
			continue
		}
		if exists {
			skipped++
			continue
		}
		_, err = pjsk.Alias.Create().
			SetAliasType("music").
			SetAliasTypeID(rec.MusicID).
			SetAlias(rec.Alias).
			Save(ctx)
		if err != nil {
			if pjskDB.IsConstraintError(err) {
				skipped++
				continue
			}
			log.Printf("[music-aliases] WARN record %d: insert: %v", rec.ID, err)
			failed++
			continue
		}
		inserted++
	}
	log.Printf("[music-aliases] inserted=%d skipped=%d failed=%d", inserted, skipped, failed)
	return nil
}

func importGroupAliases(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, dryRun bool) error {
	records, err := loadJSON[groupCharAliasRecord](exportsDir + "/group_character_alias.json")
	if err != nil {
		return err
	}
	log.Printf("[group-aliases] %d records", len(records))

	inserted, skipped, failed := 0, 0, 0
	for _, rec := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if dryRun {
			inserted++
			continue
		}
		exists, err := pjsk.GroupAlias.Query().
			Where(
				groupalias.Platform("qq"),
				groupalias.GroupID(rec.GroupID),
				groupalias.AliasType("character"),
				groupalias.AliasTypeID(rec.CharacterID),
				groupalias.AliasEQ(rec.Alias),
			).Exist(ctx)
		if err != nil {
			log.Printf("[group-aliases] WARN record %d: query: %v", rec.ID, err)
			failed++
			continue
		}
		if exists {
			skipped++
			continue
		}
		_, err = pjsk.GroupAlias.Create().
			SetPlatform("qq").
			SetGroupID(rec.GroupID).
			SetAliasType("character").
			SetAliasTypeID(rec.CharacterID).
			SetAlias(rec.Alias).
			Save(ctx)
		if err != nil {
			if pjskDB.IsConstraintError(err) {
				skipped++
				continue
			}
			log.Printf("[group-aliases] WARN record %d: insert: %v", rec.ID, err)
			failed++
			continue
		}
		inserted++
	}
	log.Printf("[group-aliases] inserted=%d skipped=%d failed=%d", inserted, skipped, failed)
	return nil
}

// ──────────────────────────────────────────────
// Config helpers
// ──────────────────────────────────────────────

func resolveDBConfig(envURL, envType, cfgURL, cfgType, defaultType string) (dbType, dsn string, err error) {
	if u := strings.TrimSpace(os.Getenv(envURL)); u != "" {
		t := strings.TrimSpace(os.Getenv(envType))
		if t == "" {
			t = defaultType
		}
		return t, u, nil
	}
	if cfgURL != "" {
		t := cfgType
		if t == "" {
			t = defaultType
		}
		return t, cfgURL, nil
	}
	return "", "", fmt.Errorf("DB URL not set (env %s or config file)", envURL)
}

// ──────────────────────────────────────────────
// main
// ──────────────────────────────────────────────

func main() {
	exportsDir := flag.String("exports-dir", "./exports", "directory containing the JSON export files")
	targetFlag := flag.String("target", "all", "comma-separated targets: all, bindings, character-aliases, music-aliases, group-aliases, defaults")
	dryRun := flag.Bool("dry-run", false, "parse and count records without writing to DB")
	configPath := flag.String("config", "haruki-cloud.yaml", "path to haruki-cloud.yaml")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Resolve targets
	targets := map[string]bool{}
	for _, t := range strings.Split(*targetFlag, ",") {
		targets[strings.TrimSpace(t)] = true
	}
	runAll := targets["all"]
	runBindings := runAll || targets["bindings"]
	runCharAliases := runAll || targets["character-aliases"]
	runMusicAliases := runAll || targets["music-aliases"]
	runGroupAliases := runAll || targets["group-aliases"]
	runDefaults := runAll || targets["defaults"]

	if *dryRun {
		log.Println("[importer] DRY RUN mode — no writes will be performed")
	}

	// Load config if available
	var cfg *harukiConfig.Config
	if _, err := os.Stat(*configPath); err == nil {
		loaded, err := harukiConfig.ReadConfig(*configPath)
		if err != nil {
			log.Fatalf("failed to read config: %v", err)
		}
		cfg = &loaded
	}

	// Open PJSK DB (needed for all targets; skipped in dry-run)
	var pjsk *pjskDB.Client
	if !*dryRun {
		var cfgPJSKURL, cfgPJSKType string
		if cfg != nil {
			cfgPJSKURL = cfg.PJSK.DBURL
			cfgPJSKType = cfg.PJSK.DBType
		}
		pjskType, pjskDSN, err := resolveDBConfig("HARUKI_PJSK_DB_URL", "HARUKI_PJSK_DB_TYPE", cfgPJSKURL, cfgPJSKType, "postgres")
		if err != nil {
			log.Fatalf("PJSK DB: %v", err)
		}
		pjsk, err = pjskDB.Open(pjskType, pjskDSN)
		if err != nil {
			log.Fatalf("open PJSK DB: %v", err)
		}
		defer pjsk.Close()
		if err := pjsk.Schema.Create(ctx); err != nil {
			log.Fatalf("migrate PJSK schema: %v", err)
		}
	}

	// Open Users DB (needed only for bindings; skipped in dry-run)
	var users *usersDB.Client
	if (runBindings || runDefaults) && !*dryRun {
		var cfgUsersURL, cfgUsersType string
		if cfg != nil {
			cfgUsersURL = cfg.UsersDB.DBURL
			cfgUsersType = cfg.UsersDB.DBType
		}
		usersType, usersDSN, err := resolveDBConfig("HARUKI_USERS_DB_URL", "HARUKI_USERS_DB_TYPE", cfgUsersURL, cfgUsersType, "postgres")
		if err != nil {
			log.Fatalf("Users DB: %v", err)
		}
		users, err = usersDB.Open(usersType, usersDSN)
		if err != nil {
			log.Fatalf("open Users DB: %v", err)
		}
		defer users.Close()
		if err := users.Schema.Create(ctx); err != nil {
			log.Fatalf("migrate Users schema: %v", err)
		}

		// Warm up: confirm users table is accessible
		if _, err := users.User.Query().Where(user.Platform("__probe__")).Exist(ctx); err != nil {
			log.Fatalf("users DB probe: %v", err)
		}
	}

	// ── Run targets ──────────────────────────────

	if runBindings {
		if err := importBindings(ctx, *exportsDir, pjsk, users, *dryRun); err != nil {
			log.Fatalf("[bindings] fatal: %v", err)
		}
	}
	if runCharAliases {
		if err := importCharacterAliases(ctx, *exportsDir, pjsk, *dryRun); err != nil {
			log.Fatalf("[character-aliases] fatal: %v", err)
		}
	}
	if runMusicAliases {
		if err := importMusicAliases(ctx, *exportsDir, pjsk, *dryRun); err != nil {
			log.Fatalf("[music-aliases] fatal: %v", err)
		}
	}
	if runGroupAliases {
		if err := importGroupAliases(ctx, *exportsDir, pjsk, *dryRun); err != nil {
			log.Fatalf("[group-aliases] fatal: %v", err)
		}
	}
	// defaults must run after bindings are in place.
	if runDefaults {
		if err := setDefaultBindings(ctx, pjsk, *dryRun); err != nil {
			log.Fatalf("[defaults] fatal: %v", err)
		}
	}

	log.Println("[importer] done")
}
