package accountdata

import (
	"context"
	"database/sql"
	"fmt"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"

	"entgo.io/ent/dialect"
)

// Exposure is one way a bound game account is shown to people other than
// its owner. The owner hides or shows each one separately; suite and MySekai
// data have their own flags (SuiteVisible, MySekaiVisible), which also block
// the owner's own queries.
type Exposure string

const (
	// ExposureUID is the full game UID in replies and images (uid_visible).
	// Hidden, the UID is masked; nothing else is refused.
	ExposureUID Exposure = "uid"
	// ExposureSK is another user's event ranking lookup of this account via
	// @ (sk_visible).
	ExposureSK Exposure = "sk"
	// ExposureProfile is another user's view of this account via @: the
	// profile card and every query built from the account's data
	// (profile_visible).
	ExposureProfile Exposure = "profile"
	// ExposureArrest is another user's arrest lookup of this account via @
	// (arrest_visible).
	ExposureArrest Exposure = "arrest"
)

// Exposures lists the per-exposure settings in display order.
var Exposures = []Exposure{ExposureUID, ExposureSK, ExposureProfile, ExposureArrest}

// Visibility holds a binding's per-exposure settings: true means shown.
type Visibility struct {
	UID     bool
	SK      bool
	Profile bool
	Arrest  bool
}

// UniformVisibility shows (true) or hides (false) every exposure, which is
// what the single legacy visible flag meant.
func UniformVisibility(visible bool) Visibility {
	return Visibility{UID: visible, SK: visible, Profile: visible, Arrest: visible}
}

// Allows reports whether exposure e is shown.
func (v Visibility) Allows(e Exposure) bool {
	switch e {
	case ExposureUID:
		return v.UID
	case ExposureSK:
		return v.SK
	case ExposureProfile:
		return v.Profile
	case ExposureArrest:
		return v.Arrest
	default:
		return false
	}
}

// With returns v with exposure e set to shown.
func (v Visibility) With(e Exposure, shown bool) Visibility {
	switch e {
	case ExposureUID:
		v.UID = shown
	case ExposureSK:
		v.SK = shown
	case ExposureProfile:
		v.Profile = shown
	case ExposureArrest:
		v.Arrest = shown
	}
	return v
}

// All reports whether every exposure is shown.
func (v Visibility) All() bool {
	return v.UID && v.SK && v.Profile && v.Arrest
}

// bindingVisibility reads b's per-exposure flags. Every write sets all four,
// and DropLegacyVisibleColumn backfills the rows written before the split, so
// a NULL flag only appears on a row written by a binary older than 3.9.0. It
// reads as hidden: never show more than the owner may have chosen.
func bindingVisibility(b *pjskdb.UserBinding) Visibility {
	if b == nil {
		return Visibility{}
	}
	flag := func(value *bool) bool {
		return value != nil && *value
	}
	return Visibility{
		UID:     flag(b.UIDVisible),
		SK:      flag(b.SkVisible),
		Profile: flag(b.ProfileVisible),
		Arrest:  flag(b.ArrestVisible),
	}
}

// setBindingVisibility writes every per-exposure flag of v, so a row never
// mixes set and NULL flags after a change.
func setBindingVisibility(update *pjskdb.UserBindingUpdateOne, v Visibility) *pjskdb.UserBindingUpdateOne {
	return update.
		SetUIDVisible(v.UID).
		SetSkVisible(v.SK).
		SetProfileVisible(v.Profile).
		SetArrestVisible(v.Arrest)
}

// createBindingVisibility is setBindingVisibility for a new binding.
func createBindingVisibility(create *pjskdb.UserBindingCreate, v Visibility) *pjskdb.UserBindingCreate {
	return create.
		SetUIDVisible(v.UID).
		SetSkVisible(v.SK).
		SetProfileVisible(v.Profile).
		SetArrestVisible(v.Arrest)
}

// NewBindingVisibility is the visibility a newly bound account is created
// with; every creation path sets it explicitly, so a binding never has NULL
// flags. Only the UID is hidden, which is what "Hide bound account IDs by
// default" meant; ranking, profile and arrest lookups are shown until the
// owner hides them.
var NewBindingVisibility = Visibility{UID: false, SK: true, Profile: true, Arrest: true}

// legacyVisibleColumns are the per-exposure columns that took their first
// value from the legacy user_bindings.visible column.
var legacyVisibleColumns = []string{
	userbinding.FieldUIDVisible,
	userbinding.FieldSkVisible,
	userbinding.FieldProfileVisible,
	userbinding.FieldArrestVisible,
}

// DropLegacyVisibleColumn removes the deprecated user_bindings.visible column
// (replaced by the per-exposure flags in 3.9.0). It runs after the PJSK
// auto-migrate on a writable node; ent's auto-migrate never drops a column on
// its own. When the column still exists it first copies it into every
// per-exposure flag that is still NULL (the 3.9.0 bootstrap, run one last
// time), then drops it, in one transaction where the dialect allows. When the
// column is already gone it does nothing, so it is safe on every start.
// It is not additive: no older API-role process may still run against the
// database (it writes visible on every binding insert), so the upgrade must
// stop the old instance before the new one starts.
//
// After it has run, a rollback below 3.9.0 is no longer possible: such a
// binary only reads visible, and its auto-migrate would re-add the column
// with the default true, showing every account. Rolling back to 3.9.0-3.11.x
// stays safe: they re-add the column, but read it only for NULL flags, and
// there are none.
//
// It returns the number of flag values backfilled and whether the column was
// dropped.
func DropLegacyVisibleColumn(ctx context.Context, db *sql.DB, dialectName string) (backfilled int64, dropped bool, err error) {
	if db == nil {
		return 0, false, nil
	}
	exists, err := legacyVisibleColumnExists(ctx, db, dialectName)
	if err != nil || !exists {
		return 0, false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("drop user_bindings.visible: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, col := range legacyVisibleColumns {
		res, execErr := tx.ExecContext(ctx, "UPDATE "+userbinding.Table+" SET "+col+" = visible WHERE "+col+" IS NULL")
		if execErr != nil {
			return backfilled, false, fmt.Errorf("backfill user_bindings.%s: %w", col, execErr)
		}
		if n, rowsErr := res.RowsAffected(); rowsErr == nil {
			backfilled += n
		}
	}
	if _, err = tx.ExecContext(ctx, "ALTER TABLE "+userbinding.Table+" DROP COLUMN visible"); err != nil {
		return backfilled, false, fmt.Errorf("drop user_bindings.visible: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return backfilled, false, fmt.Errorf("drop user_bindings.visible: commit: %w", err)
	}
	return backfilled, true, nil
}

func legacyVisibleColumnExists(ctx context.Context, db *sql.DB, dialectName string) (bool, error) {
	query, err := legacyVisibleColumnQuery(dialectName)
	if err != nil {
		return false, err
	}
	var n int
	if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return false, fmt.Errorf("look up user_bindings.visible: %w", err)
	}
	return n > 0, nil
}

// legacyVisibleColumnQuery returns a query counting the visible column of
// user_bindings in the current schema.
func legacyVisibleColumnQuery(dialectName string) (string, error) {
	switch dialectName {
	case dialect.Postgres:
		return "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'user_bindings' AND column_name = 'visible'", nil
	case dialect.MySQL:
		return "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'user_bindings' AND column_name = 'visible'", nil
	case dialect.SQLite:
		return "SELECT COUNT(*) FROM pragma_table_info('user_bindings') WHERE name = 'visible'", nil
	default:
		return "", fmt.Errorf("drop user_bindings.visible: unsupported dialect %q", dialectName)
	}
}
