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
	"haruki-cloud/utils/logger"

	"haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/gameaccount"
	"haruki-cloud/database/pjsk/groupalias"
	"haruki-cloud/database/pjsk/userbinding"

	_ "github.com/lib/pq"
	json "haruki-cloud/internal/jsonutil"
)

var importerLogger = logger.NewLoggerFromGlobal("Importer")

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
	if err := json.NewDecoder(f).Decode(&records); err != nil {
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

func importErrorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

func logRecordFailure(ctx context.Context, target, region, operation string, recordID int, err error) {
	importerLogger.WarnContext(ctx, "import record skipped",
		"target", target,
		"region", region,
		"operation", operation,
		"record_id", recordID,
		"error_type", importErrorType(err),
	)
}

func exitImportFailure(target, operation string, err error) {
	importerLogger.Error("importer stopped",
		"target", target,
		"operation", operation,
		"error_type", importErrorType(err),
	)
	os.Exit(1)
}

// ──────────────────────────────────────────────
// Import functions
// ──────────────────────────────────────────────

type importCounts struct {
	total    int
	inserted int
	skipped  int
	failed   int
}

type bindingImportOutcome int

const (
	bindingInserted bindingImportOutcome = iota
	bindingSkipped
	bindingFailed
)

func importBindings(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, users *usersDB.Client, dryRun bool) error {
	files := []string{"bind.json", "cn_bind.json", "en_bind.json", "tw_bind.json", "kr_bind.json"}
	resolver := identity.NewResolver(users)
	counts := importCounts{}
	for _, filename := range files {
		if err := importBindingFile(ctx, exportsDir, filename, pjsk, resolver, dryRun, &counts); err != nil {
			return err
		}
	}
	importerLogger.InfoContext(ctx, importTargetCompleted,
		"target", "bindings",
		"total", counts.total,
		"inserted", counts.inserted,
		"skipped", counts.skipped,
		"failed", counts.failed,
	)
	return nil
}

func importBindingFile(ctx context.Context, exportsDir, filename string, pjsk *pjskDB.Client, resolver *identity.Resolver, dryRun bool, counts *importCounts) error {
	path := exportsDir + "/" + filename
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		importerLogger.Info("import source skipped", "target", "bindings", "file", filename, "reason", "file_not_found")
		return nil
	}
	records, err := loadJSON[bindRecord](path)
	if err != nil {
		return fmt.Errorf("load %s: %w", filename, err)
	}
	server := serverFromFilename(filename)
	importerLogger.Info("import source loaded", "target", "bindings", "file", filename, "region", server, "records", len(records))
	for i, rec := range records {
		counts.total++
		if err := ctx.Err(); err != nil {
			return err
		}
		if dryRun {
			logBindingDryRunProgress(ctx, server, i, len(records))
			counts.inserted++
			continue
		}
		counts.record(importBindingRecord(ctx, server, rec, pjsk, resolver))
	}
	importerLogger.InfoContext(ctx, "import source completed", "target", "bindings", "region", server)
	return nil
}

func logBindingDryRunProgress(ctx context.Context, server string, index, total int) {
	if (index+1)%5000 != 0 {
		return
	}
	importerLogger.InfoContext(ctx, "import progress", "target", "bindings", "region", server, "dry_run", true, "processed", index+1, "total", total)
}

func (c *importCounts) record(outcome bindingImportOutcome) {
	switch outcome {
	case bindingInserted:
		c.inserted++
	case bindingSkipped:
		c.skipped++
	case bindingFailed:
		c.failed++
	}
}

func importBindingRecord(ctx context.Context, server string, rec bindRecord, pjsk *pjskDB.Client, resolver *identity.Resolver) bindingImportOutcome {
	platform, platformUserID := detectPlatform(rec.IMUserID)
	harukiUserID, err := resolver.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return failedBindingRecord(ctx, server, "resolve_user", rec.ID, err)
	}
	gameAccountID, operation, err := resolveBindingGameAccountID(ctx, pjsk, server, rec.UserID)
	if err != nil {
		return failedBindingRecord(ctx, server, operation, rec.ID, err)
	}
	created, operation, err := createBindingIfMissing(ctx, pjsk, harukiUserID, gameAccountID, rec.IsPrivate == 0)
	if err != nil {
		return failedBindingRecord(ctx, server, operation, rec.ID, err)
	}
	if !created {
		return bindingSkipped
	}
	return bindingInserted
}

func failedBindingRecord(ctx context.Context, server, operation string, recordID int, err error) bindingImportOutcome {
	logRecordFailure(ctx, "bindings", server, operation, recordID, err)
	return bindingFailed
}

func resolveBindingGameAccountID(ctx context.Context, pjsk *pjskDB.Client, server, gameUserID string) (int, string, error) {
	gameAcc, err := pjsk.GameAccount.Query().Where(gameaccount.ServerEQ(server), gameaccount.UserIDEQ(gameUserID)).Only(ctx)
	if err == nil {
		return gameAcc.ID, "", nil
	}
	if !pjskDB.IsNotFound(err) {
		return 0, "query_game_account", err
	}
	gameAcc, err = pjsk.GameAccount.Create().SetServer(server).SetUserID(gameUserID).Save(ctx)
	if pjskDB.IsConstraintError(err) {
		gameAcc, err = pjsk.GameAccount.Query().Where(gameaccount.ServerEQ(server), gameaccount.UserIDEQ(gameUserID)).Only(ctx)
	}
	if err != nil {
		return 0, "create_game_account", err
	}
	return gameAcc.ID, "", nil
}

func createBindingIfMissing(ctx context.Context, pjsk *pjskDB.Client, harukiUserID, gameAccountID int, visible bool) (bool, string, error) {
	exists, err := pjsk.UserBinding.Query().Where(userbinding.HarukiUserID(harukiUserID), userbinding.GameAccountIDEQ(gameAccountID)).Exist(ctx)
	if err != nil {
		return false, "query_binding", err
	}
	if exists {
		return false, "", nil
	}
	displayOrder, err := nextDisplayOrder(ctx, pjsk, harukiUserID)
	if err != nil {
		return false, "display_order", err
	}
	_, err = pjsk.UserBinding.Create().SetHarukiUserID(harukiUserID).SetGameAccountID(gameAccountID).SetDisplayOrder(displayOrder).SetVisible(visible).Save(ctx)
	if pjskDB.IsConstraintError(err) {
		return false, "", nil
	}
	return err == nil, "create_binding", err
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

type defaultBindingInfo struct {
	bindingID int
	server    string
}

type defaultBindingScopeKey struct {
	userID int
	server string
}

type defaultBindingImportState struct {
	ctx        context.Context
	pjsk       *pjskDB.Client
	alreadySet map[defaultBindingScopeKey]bool
	inserted   int
	skipped    int
	failed     int
}

// setDefaultBindings creates UserDefaultBinding rows for every user:
//   - 1 binding total   → global default ("default") + server default
//   - multiple bindings → global default by priority (jp>cn>tw>en>kr); per-server default
//
// Existing defaults are skipped (idempotent).
func setDefaultBindings(ctx context.Context, pjsk *pjskDB.Client, dryRun bool) error {
	if dryRun {
		importerLogger.InfoContext(ctx, "import target skipped",
			"target", "defaults",
			"dry_run", true,
		)
		return nil
	}
	// Load all bindings with their game accounts.
	allBindings, err := pjsk.UserBinding.Query().
		WithGameAccount().
		All(ctx)
	if err != nil {
		return fmt.Errorf("query bindings: %w", err)
	}

	byUser := groupBindingsByUser(allBindings)

	importerLogger.InfoContext(ctx, importTargetLoaded,
		"target", "defaults",
		"users", len(byUser),
	)

	// Load existing defaults to avoid redundant writes.
	existingDefaults, err := pjsk.UserDefaultBinding.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("query existing defaults: %w", err)
	}
	alreadySet := make(map[defaultBindingScopeKey]bool, len(existingDefaults))
	for _, d := range existingDefaults {
		alreadySet[defaultBindingScopeKey{d.HarukiUserID, d.Server}] = true
	}
	state := &defaultBindingImportState{ctx: ctx, pjsk: pjsk, alreadySet: alreadySet}
	for harukiUserID, binds := range byUser {
		if err := ctx.Err(); err != nil {
			return err
		}
		state.importUser(harukiUserID, binds)
	}

	importerLogger.InfoContext(ctx, importTargetCompleted,
		"target", "defaults",
		"inserted", state.inserted,
		"skipped", state.skipped,
		"failed", state.failed,
	)
	return nil
}

func groupBindingsByUser(allBindings []*pjskDB.UserBinding) map[int][]defaultBindingInfo {
	byUser := make(map[int][]defaultBindingInfo, len(allBindings)/2)
	for _, binding := range allBindings {
		if binding.Edges.GameAccount == nil {
			continue
		}
		byUser[binding.HarukiUserID] = append(byUser[binding.HarukiUserID], defaultBindingInfo{
			bindingID: binding.ID,
			server:    binding.Edges.GameAccount.Server,
		})
	}
	return byUser
}

func (s *defaultBindingImportState) create(harukiUserID, bindingID int, scope string) {
	key := defaultBindingScopeKey{harukiUserID, scope}
	if s.alreadySet[key] {
		s.skipped++
		return
	}
	_, err := s.pjsk.UserDefaultBinding.Create().SetHarukiUserID(harukiUserID).SetServer(scope).SetBindingID(bindingID).Save(s.ctx)
	if pjskDB.IsConstraintError(err) {
		s.skipped++
		return
	}
	if err != nil {
		importerLogger.WarnContext(s.ctx, "default binding skipped", "target", "defaults", "scope", scope, "operation", "create_default", "error_type", importErrorType(err))
		s.failed++
		return
	}
	s.alreadySet[key] = true
	s.inserted++
}

func (s *defaultBindingImportState) importUser(harukiUserID int, binds []defaultBindingInfo) {
	const globalDefaultScope = "default"
	if len(binds) == 1 {
		s.create(harukiUserID, binds[0].bindingID, globalDefaultScope)
		s.create(harukiUserID, binds[0].bindingID, binds[0].server)
		return
	}
	byServer := firstBindingByServer(binds)
	for _, server := range globalServerPriority {
		if bindingID, ok := byServer[server]; ok {
			s.create(harukiUserID, bindingID, globalDefaultScope)
			break
		}
	}
	for server, bindingID := range byServer {
		s.create(harukiUserID, bindingID, server)
	}
}

func firstBindingByServer(binds []defaultBindingInfo) map[string]int {
	result := make(map[string]int)
	for _, binding := range binds {
		if _, exists := result[binding.server]; !exists {
			result[binding.server] = binding.bindingID
		}
	}
	return result
}

func importCharacterAliases(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, dryRun bool) error {
	records, err := loadJSON[charAliasRecord](exportsDir + "/character_alias.json")
	if err != nil {
		return err
	}
	importerLogger.InfoContext(ctx, importTargetLoaded, "target", characterAliases, "records", len(records))

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
			logRecordFailure(ctx, characterAliases, "", "query", rec.ID, err)
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
			logRecordFailure(ctx, characterAliases, "", "insert", rec.ID, err)
			failed++
			continue
		}
		inserted++
	}
	importerLogger.InfoContext(ctx, importTargetCompleted,
		"target", characterAliases,
		"inserted", inserted,
		"skipped", skipped,
		"failed", failed,
	)
	return nil
}

func importMusicAliases(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, dryRun bool) error {
	records, err := loadJSON[musicAliasRecord](exportsDir + "/music_alias.json")
	if err != nil {
		return err
	}
	importerLogger.InfoContext(ctx, importTargetLoaded, "target", musicAliases, "records", len(records))

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
			logRecordFailure(ctx, musicAliases, "", "query", rec.ID, err)
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
			logRecordFailure(ctx, musicAliases, "", "insert", rec.ID, err)
			failed++
			continue
		}
		inserted++
	}
	importerLogger.InfoContext(ctx, importTargetCompleted,
		"target", musicAliases,
		"inserted", inserted,
		"skipped", skipped,
		"failed", failed,
	)
	return nil
}

func importGroupAliases(ctx context.Context, exportsDir string, pjsk *pjskDB.Client, dryRun bool) error {
	records, err := loadJSON[groupCharAliasRecord](exportsDir + "/group_character_alias.json")
	if err != nil {
		return err
	}
	importerLogger.InfoContext(ctx, importTargetLoaded, "target", groupAliases, "records", len(records))

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
			logRecordFailure(ctx, groupAliases, "", "query", rec.ID, err)
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
			logRecordFailure(ctx, groupAliases, "", "insert", rec.ID, err)
			failed++
			continue
		}
		inserted++
	}
	importerLogger.InfoContext(ctx, importTargetCompleted,
		"target", groupAliases,
		"inserted", inserted,
		"skipped", skipped,
		"failed", failed,
	)
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

type importerTargets struct {
	bindings        bool
	characterAlias  bool
	musicAlias      bool
	groupAlias      bool
	defaultBindings bool
}

func main() {
	logger.SetGlobalFileWriter(os.Stdout)
	logger.SetGlobalLogLevel(harukiConfig.LogLevelInfo)
	logger.InstallStandardHandlers()
	exportsDir := flag.String("exports-dir", "./exports", "directory containing the JSON export files")
	targetFlag := flag.String("target", "all", "comma-separated targets: all, bindings, character-aliases, music-aliases, group-aliases, defaults")
	dryRun := flag.Bool("dry-run", false, "parse and count records without writing to DB")
	configPath := flag.String("config", "haruki-cloud.yaml", "path to haruki-cloud.yaml")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	targets := parseImporterTargets(*targetFlag)
	if *dryRun {
		importerLogger.Info("importer started", "dry_run", true)
	}
	cfg := loadImporterConfig(*configPath)
	pjsk := openImporterPJSK(ctx, cfg, *dryRun)
	if pjsk != nil {
		defer pjsk.Close()
	}
	users := openImporterUsers(ctx, cfg, targets.bindings || targets.defaultBindings, *dryRun)
	if users != nil {
		defer users.Close()
	}
	runImporterTargets(ctx, targets, *exportsDir, pjsk, users, *dryRun)
	importerLogger.Info("importer completed", "dry_run", *dryRun)
}

func parseImporterTargets(value string) importerTargets {
	selected := make(map[string]bool)
	for _, target := range strings.Split(value, ",") {
		selected[strings.TrimSpace(target)] = true
	}
	all := selected["all"]
	return importerTargets{
		bindings:        all || selected["bindings"],
		characterAlias:  all || selected[characterAliases],
		musicAlias:      all || selected[musicAliases],
		groupAlias:      all || selected[groupAliases],
		defaultBindings: all || selected["defaults"],
	}
}

func loadImporterConfig(path string) *harukiConfig.Config {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	loaded, err := harukiConfig.ReadConfig(path)
	if err != nil {
		exitImportFailure("config", "read", err)
		return nil
	}
	return &loaded
}

func openImporterPJSK(ctx context.Context, cfg *harukiConfig.Config, dryRun bool) *pjskDB.Client {
	if dryRun {
		return nil
	}
	var cfgURL, cfgType string
	if cfg != nil {
		cfgURL, cfgType = cfg.PJSK.DBURL, cfg.PJSK.DBType
	}
	dbType, dsn, err := resolveDBConfig("HARUKI_PJSK_DB_URL", "HARUKI_PJSK_DB_TYPE", cfgURL, cfgType, "postgres")
	if err != nil {
		exitImportFailure("pjsk_db", "resolve_config", err)
		return nil
	}
	client, err := pjskDB.Open(dbType, dsn)
	if err != nil {
		exitImportFailure("pjsk_db", "open", err)
		return nil
	}
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		exitImportFailure("pjsk_db", "migrate", err)
		return nil
	}
	return client
}

func openImporterUsers(ctx context.Context, cfg *harukiConfig.Config, needed, dryRun bool) *usersDB.Client {
	if !needed || dryRun {
		return nil
	}
	var cfgURL, cfgType string
	if cfg != nil {
		cfgURL, cfgType = cfg.UsersDB.DBURL, cfg.UsersDB.DBType
	}
	dbType, dsn, err := resolveDBConfig("HARUKI_USERS_DB_URL", "HARUKI_USERS_DB_TYPE", cfgURL, cfgType, "postgres")
	if err != nil {
		exitImportFailure("users_db", "resolve_config", err)
		return nil
	}
	client, err := usersDB.Open(dbType, dsn)
	if err != nil {
		exitImportFailure("users_db", "open", err)
		return nil
	}
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		exitImportFailure("users_db", "migrate", err)
		return nil
	}
	if _, err := client.User.Query().Where(user.Platform("__probe__")).Exist(ctx); err != nil {
		client.Close()
		exitImportFailure("users_db", "probe", err)
		return nil
	}
	return client
}

func runImporterTargets(ctx context.Context, targets importerTargets, exportsDir string, pjsk *pjskDB.Client, users *usersDB.Client, dryRun bool) {
	runs := []struct {
		enabled bool
		name    string
		run     func() error
	}{
		{targets.bindings, "bindings", func() error { return importBindings(ctx, exportsDir, pjsk, users, dryRun) }},
		{targets.characterAlias, characterAliases, func() error { return importCharacterAliases(ctx, exportsDir, pjsk, dryRun) }},
		{targets.musicAlias, musicAliases, func() error { return importMusicAliases(ctx, exportsDir, pjsk, dryRun) }},
		{targets.groupAlias, groupAliases, func() error { return importGroupAliases(ctx, exportsDir, pjsk, dryRun) }},
		{targets.defaultBindings, "defaults", func() error { return setDefaultBindings(ctx, pjsk, dryRun) }},
	}
	for _, target := range runs {
		if !target.enabled {
			continue
		}
		if err := target.run(); err != nil {
			exitImportFailure(target.name, "run", err)
		}
	}
}
