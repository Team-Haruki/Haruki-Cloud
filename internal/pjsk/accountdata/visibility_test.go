package accountdata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	"haruki-cloud/internal/i18n"

	"entgo.io/ent/dialect"
)

// openLegacyVisibleDB opens a PJSK database whose user_bindings table still
// has the pre-3.9.0 visible column, as an older binary created it.
func openLegacyVisibleDB(t *testing.T, name string) (*pjskdb.Client, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:accountdata_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano())
	client := pjskenttest.Open(t, "sqlite3", dsn)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("ALTER TABLE user_bindings ADD COLUMN visible bool NOT NULL DEFAULT true"); err != nil {
		t.Fatalf("add legacy column: %v", err)
	}
	return client, db
}

// createLegacyBinding writes a row as a binary before the split did: only
// visible is set, the per-exposure flags are NULL.
func createLegacyBinding(t *testing.T, ctx context.Context, client *pjskdb.Client, db *sql.DB, harukiUserID int, uid string, visible bool) *pjskdb.UserBinding {
	t.Helper()
	account, err := client.GameAccount.Create().SetServer("jp").SetUserID(uid).Save(ctx)
	if err != nil {
		t.Fatalf("create account %s: %v", uid, err)
	}
	binding, err := client.UserBinding.Create().SetHarukiUserID(harukiUserID).SetGameAccountID(account.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create binding %s: %v", uid, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE user_bindings SET visible = ? WHERE id = ?", visible, binding.ID); err != nil {
		t.Fatalf("set legacy visible %s: %v", uid, err)
	}
	return binding
}

func reloadVisibility(t *testing.T, ctx context.Context, client *pjskdb.Client, id int) (*pjskdb.UserBinding, Visibility) {
	t.Helper()
	row, err := client.UserBinding.Get(ctx, id)
	if err != nil {
		t.Fatalf("reload binding %d: %v", id, err)
	}
	return row, bindingVisibility(row)
}

func legacyColumnCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('user_bindings') WHERE name = 'visible'").Scan(&n); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	return n
}

// TestDropLegacyVisibleColumn: before the drop, every flag still NULL takes
// the legacy value (hidden stays hidden for every exposure, shown stays
// shown), a flag already set is kept, then the column is gone; a second run
// is a no-op.
func TestDropLegacyVisibleColumn(t *testing.T) {
	ctx := context.Background()
	client, db := openLegacyVisibleDB(t, "visible_drop")
	hidden := createLegacyBinding(t, ctx, client, db, 1, "1001", false)
	shown := createLegacyBinding(t, ctx, client, db, 2, "1002", true)
	partial := createLegacyBinding(t, ctx, client, db, 3, "1003", false)
	if _, err := client.UserBinding.UpdateOneID(partial.ID).SetUIDVisible(true).Save(ctx); err != nil {
		t.Fatalf("set partial flag: %v", err)
	}
	account, err := client.GameAccount.Create().SetServer("jp").SetUserID("1004").Save(ctx)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	fresh, err := createBindingVisibility(client.UserBinding.Create().SetHarukiUserID(4).SetGameAccountID(account.ID), NewBindingVisibility).Save(ctx)
	if err != nil {
		t.Fatalf("create new binding: %v", err)
	}

	backfilled, dropped, err := DropLegacyVisibleColumn(ctx, db, dialect.SQLite)
	if err != nil || !dropped {
		t.Fatalf("drop = %d, %v, %v", backfilled, dropped, err)
	}
	if want := int64(4 + 4 + 3); backfilled != want {
		t.Fatalf("backfilled %d flag values, want %d", backfilled, want)
	}
	if n := legacyColumnCount(t, db); n != 0 {
		t.Fatalf("visible column still present (%d)", n)
	}
	for _, tc := range []struct {
		id   int
		want Visibility
	}{
		{hidden.ID, UniformVisibility(false)},
		{shown.ID, UniformVisibility(true)},
		{partial.ID, Visibility{UID: true}},
		{fresh.ID, NewBindingVisibility},
	} {
		row, v := reloadVisibility(t, ctx, client, tc.id)
		if v != tc.want {
			t.Fatalf("binding %d after drop = %+v, want %+v", tc.id, v, tc.want)
		}
		if row.UIDVisible == nil || row.SkVisible == nil || row.ProfileVisible == nil || row.ArrestVisible == nil {
			t.Fatalf("binding %d still has NULL flags: %+v", tc.id, row)
		}
	}

	if n, again, err := DropLegacyVisibleColumn(ctx, db, dialect.SQLite); err != nil || again || n != 0 {
		t.Fatalf("second run = %d, %v, %v", n, again, err)
	}
	if n, again, err := DropLegacyVisibleColumn(ctx, nil, dialect.SQLite); err != nil || again || n != 0 {
		t.Fatalf("nil db = %d, %v, %v", n, again, err)
	}
	if _, _, err := DropLegacyVisibleColumn(ctx, db, "oracle"); err == nil {
		t.Fatal("unsupported dialect accepted")
	}
}

// TestVisibilityCommandsChangeOnlyTheirExposure runs the visibility commands
// on one binding and checks each changes its own exposure and that the status
// command writes nothing.
func TestVisibilityCommandsChangeOnlyTheirExposure(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "visibility_modes", accountCoverageValidator{profiles: map[string]string{"jp": "JP Player"}})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	items, err := service.List(ctx, "qq", "42")
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %+v, %v", items, err)
	}
	bindingID := items[0].BindingID
	if items[0].Visibility != NewBindingVisibility {
		t.Fatalf("new binding visibility = %+v, want only the UID hidden", items[0].Visibility)
	}
	params := ProfileSettingsCommandParams{Platform: "qq", PlatformUserID: "42", Server: "jp", RegionExplicit: true}
	run := func(mode string) string {
		t.Helper()
		reply, err := ExecuteProfileSettingsCommand(ctx, service, mode, params)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		return string(reply)
	}

	steps := []struct {
		mode string
		want Visibility
	}{
		{ProfileModeShowAll, UniformVisibility(true)},
		{ProfileModeHideID, Visibility{UID: false, SK: true, Profile: true, Arrest: true}},
		{ProfileModeHideSK, Visibility{UID: false, SK: false, Profile: true, Arrest: true}},
		{ProfileModeShowID, Visibility{UID: true, SK: false, Profile: true, Arrest: true}},
		{ProfileModeHideInfo, Visibility{UID: true, SK: false, Profile: false, Arrest: true}},
		{ProfileModeHideArrest, Visibility{UID: true, SK: false, Profile: false, Arrest: false}},
		{ProfileModeShowSK, Visibility{UID: true, SK: true, Profile: false, Arrest: false}},
		{ProfileModeShowInfo, Visibility{UID: true, SK: true, Profile: true, Arrest: false}},
		{ProfileModeShowArrest, UniformVisibility(true)},
		{ProfileModeHideAll, UniformVisibility(false)},
	}
	for _, step := range steps {
		reply := run(step.mode)
		row, got := reloadVisibility(t, ctx, client, bindingID)
		if got != step.want {
			t.Fatalf("after %s visibility = %+v, want %+v", step.mode, got, step.want)
		}
		if row.UIDVisible == nil || row.SkVisible == nil || row.ProfileVisible == nil || row.ArrestVisible == nil {
			t.Fatalf("after %s a flag is NULL: %+v", step.mode, row)
		}
		if !strings.Contains(reply, i18n.T("account.visibility.state.hidden")) && step.want != UniformVisibility(true) {
			t.Fatalf("after %s reply lists no settings: %q", step.mode, reply)
		}
	}

	// Suite and MySekai keep their own toggles; /隐藏全部 does not touch them.
	if row, _ := reloadVisibility(t, ctx, client, bindingID); !row.SuiteVisible || !row.MysekaiVisible {
		t.Fatalf("hide all changed suite/mysekai: %+v", row)
	}

	before, _ := reloadVisibility(t, ctx, client, bindingID)
	status := run(ProfileModeVisibility)
	after, _ := reloadVisibility(t, ctx, client, bindingID)
	if *before.UIDVisible != *after.UIDVisible || *before.SkVisible != *after.SkVisible {
		t.Fatalf("status command changed the binding: %+v -> %+v", before, after)
	}
	for _, label := range []string{"游戏 UID", "活动排名", "个人信息", "逮捕信息", "抓包数据", "烤森数据"} {
		if !strings.Contains(status, label) {
			t.Fatalf("status reply misses %s: %q", label, status)
		}
	}
	if profileSettingsModeMutates(ProfileModeVisibility, params) {
		t.Fatal("the status command must not need a writable node")
	}
}

func TestVisibilityHelpers(t *testing.T) {
	v := UniformVisibility(true)
	for _, exposure := range Exposures {
		if !v.Allows(exposure) || v.With(exposure, false).Allows(exposure) || !v.With(exposure, false).With(exposure, true).All() {
			t.Fatalf("exposure %s", exposure)
		}
		if v.With(exposure, false).All() {
			t.Fatalf("All with %s hidden", exposure)
		}
	}
	if v.Allows("unknown") || v.With("unknown", false) != v {
		t.Fatal("unknown exposure")
	}
	if bindingVisibility(nil) != (Visibility{}) {
		t.Fatal("nil binding visibility")
	}
}

// TestNullFlagsReadAsHidden: a row with NULL flags (only an older binary
// writes one) reads as hidden for every exposure, and the first toggle on it
// writes all four flags.
func TestNullFlagsReadAsHidden(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "visibility_null_flags", accountCoverageValidator{profiles: map[string]string{"jp": "JP Player"}})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	items, err := service.List(ctx, "qq", "42")
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %+v, %v", items, err)
	}
	id := items[0].BindingID
	if _, err := client.UserBinding.UpdateOneID(id).
		ClearUIDVisible().ClearSkVisible().ClearProfileVisible().ClearArrestVisible().Save(ctx); err != nil {
		t.Fatalf("clear flags: %v", err)
	}
	if _, got := reloadVisibility(t, ctx, client, id); got != UniformVisibility(false) {
		t.Fatalf("NULL flags = %+v, want all hidden", got)
	}
	params := ProfileSettingsCommandParams{Platform: "qq", PlatformUserID: "42", Server: "jp", RegionExplicit: true}
	if _, err := ExecuteProfileSettingsCommand(ctx, service, ProfileModeShowSK, params); err != nil {
		t.Fatalf("show sk: %v", err)
	}
	want := Visibility{SK: true}
	row, got := reloadVisibility(t, ctx, client, id)
	if got != want || row.UIDVisible == nil || row.ProfileVisible == nil || row.ArrestVisible == nil {
		t.Fatalf("toggle on a NULL row = %+v (%+v), want every flag written as %+v", got, row, want)
	}
}

// New bindings get explicit flags at creation.
func TestNewBindingsGetExplicitVisibility(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "visibility_new_binding", accountCoverageValidator{profiles: map[string]string{"jp": "JP Player"}})
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	rows, err := client.UserBinding.Query().All(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	row := rows[0]
	if row.UIDVisible == nil || row.SkVisible == nil || row.ProfileVisible == nil || row.ArrestVisible == nil {
		t.Fatalf("new binding has NULL flags: %+v", row)
	}
	if got := bindingVisibility(row); got != NewBindingVisibility {
		t.Fatalf("new binding = %+v", got)
	}
}

func TestLegacyVisibleColumnQueryPerDialect(t *testing.T) {
	for _, d := range []string{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		query, err := legacyVisibleColumnQuery(d)
		if err != nil || !strings.Contains(query, "user_bindings") || !strings.Contains(query, "visible") {
			t.Fatalf("%s: %q, %v", d, query, err)
		}
	}
	if _, err := legacyVisibleColumnQuery("oracle"); err == nil {
		t.Fatal("unsupported dialect accepted")
	}
}

// A failed lookup, backfill or drop is reported and leaves the table as it
// was.
func TestDropLegacyVisibleColumnErrors(t *testing.T) {
	ctx := context.Background()

	_, db := openLegacyVisibleDB(t, "visible_drop_closed")
	_ = db.Close()
	if _, dropped, err := DropLegacyVisibleColumn(ctx, db, dialect.SQLite); err == nil || dropped {
		t.Fatalf("closed db = %v, %v", dropped, err)
	}

	// A visible column without the per-exposure flags fails the backfill.
	dsn := fmt.Sprintf("file:accountdata_visible_drop_bare_%d?mode=memory&cache=shared", time.Now().UnixNano())
	bare, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bare.Close() })
	if _, err := bare.Exec("CREATE TABLE user_bindings (id integer PRIMARY KEY, visible bool NOT NULL DEFAULT true)"); err != nil {
		t.Fatal(err)
	}
	if _, dropped, err := DropLegacyVisibleColumn(ctx, bare, dialect.SQLite); err == nil || dropped {
		t.Fatalf("backfill without flag columns = %v, %v", dropped, err)
	}
	if n := legacyColumnCount(t, bare); n != 1 {
		t.Fatalf("failed run dropped the column (%d)", n)
	}

	// A drop the database refuses (a view depends on the column) rolls the
	// backfill back.
	client, viewDB := openLegacyVisibleDB(t, "visible_drop_view")
	legacy := createLegacyBinding(t, ctx, client, viewDB, 1, "2001", false)
	if _, err := viewDB.Exec("CREATE VIEW legacy_visible AS SELECT id, visible FROM user_bindings"); err != nil {
		t.Fatal(err)
	}
	if _, dropped, err := DropLegacyVisibleColumn(ctx, viewDB, dialect.SQLite); err == nil || dropped {
		t.Fatalf("drop under a dependent view = %v, %v", dropped, err)
	}
	if row, _ := reloadVisibility(t, ctx, client, legacy.ID); row.UIDVisible != nil {
		t.Fatalf("backfill not rolled back: %+v", row)
	}
}
