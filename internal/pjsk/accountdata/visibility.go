package accountdata

import (
	"context"
	"fmt"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/predicate"
	"haruki-cloud/database/pjsk/userbinding"
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

// All reports whether every exposure is shown. It is the value kept in the
// deprecated visible column, so a rollback to a binary that only reads
// visible never shows more than the owner chose.
func (v Visibility) All() bool {
	return v.UID && v.SK && v.Profile && v.Arrest
}

// bindingVisibility reads b's per-exposure flags. A NULL flag (a row written
// before the split and not yet bootstrapped, or by an older binary during a
// rollout) falls back to the legacy visible column.
func bindingVisibility(b *pjskdb.UserBinding) Visibility {
	if b == nil {
		return Visibility{}
	}
	flag := func(value *bool) bool {
		if value == nil {
			return b.Visible
		}
		return *value
	}
	return Visibility{
		UID:     flag(b.UIDVisible),
		SK:      flag(b.SkVisible),
		Profile: flag(b.ProfileVisible),
		Arrest:  flag(b.ArrestVisible),
	}
}

// setBindingVisibility writes every per-exposure flag of v and the derived
// legacy visible column, so a row never mixes set and NULL flags after a
// change.
func setBindingVisibility(update *pjskdb.UserBindingUpdateOne, v Visibility) *pjskdb.UserBindingUpdateOne {
	return update.
		SetUIDVisible(v.UID).
		SetSkVisible(v.SK).
		SetProfileVisible(v.Profile).
		SetArrestVisible(v.Arrest).
		SetVisible(v.All())
}

// createBindingVisibility is setBindingVisibility for a new binding.
func createBindingVisibility(create *pjskdb.UserBindingCreate, v Visibility) *pjskdb.UserBindingCreate {
	return create.
		SetUIDVisible(v.UID).
		SetSkVisible(v.SK).
		SetProfileVisible(v.Profile).
		SetArrestVisible(v.Arrest).
		SetVisible(v.All())
}

// NewBindingVisibility is the visibility a newly bound account is created
// with; every creation path sets it explicitly, so a binding created after
// the split never has NULL flags. Only the UID is hidden, which is what
// "Hide bound account IDs by default" meant; ranking, profile and arrest
// lookups are shown until the owner hides them. The legacy visible column is
// therefore false for a new binding, so an older binary hides everything.
var NewBindingVisibility = Visibility{UID: false, SK: true, Profile: true, Arrest: true}

// BootstrapBindingVisibility is the one-time bootstrap of the per-exposure
// flags from the legacy visible column, run after every auto-migrate:
//
//   - a binding that was hidden (visible=false) gets every exposure hidden,
//     one that was shown gets every exposure shown;
//   - it only writes flags that are still NULL, so a bootstrapped value is
//     never overwritten on a later start, and a value the owner set with a
//     toggle (which writes all four flags) always wins;
//   - bindings created after the split get explicit flags at creation
//     (NewBindingVisibility), so the only NULLs left are pre-split rows and
//     rows an older binary created during a rolling deploy, which the next
//     start bootstraps the same way.
//
// It returns the number of flag values written.
func BootstrapBindingVisibility(ctx context.Context, client *pjskdb.Client) (int, error) {
	if client == nil {
		return 0, nil
	}
	type column struct {
		name  string
		isNil predicate.UserBinding
		set   func(*pjskdb.UserBindingUpdate, bool) *pjskdb.UserBindingUpdate
	}
	columns := []column{
		{userbinding.FieldUIDVisible, userbinding.UIDVisibleIsNil(), (*pjskdb.UserBindingUpdate).SetUIDVisible},
		{userbinding.FieldSkVisible, userbinding.SkVisibleIsNil(), (*pjskdb.UserBindingUpdate).SetSkVisible},
		{userbinding.FieldProfileVisible, userbinding.ProfileVisibleIsNil(), (*pjskdb.UserBindingUpdate).SetProfileVisible},
		{userbinding.FieldArrestVisible, userbinding.ArrestVisibleIsNil(), (*pjskdb.UserBindingUpdate).SetArrestVisible},
	}
	total := 0
	for _, col := range columns {
		for _, visible := range []bool{false, true} {
			n, err := col.set(client.UserBinding.Update().Where(col.isNil, userbinding.Visible(visible)), visible).Save(ctx)
			if err != nil {
				return total, fmt.Errorf("bootstrap user_bindings.%s: %w", col.name, err)
			}
			total += n
		}
	}
	return total, nil
}
