package handler

import (
	"context"
	"strings"
	"testing"

	corehandler "haruki-cloud/internal/handler"
	"haruki-cloud/internal/pjsk/accountdata"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

// bindWithVisibility binds the jp test account for platformUserID, shows
// every exposure, then applies modes (e.g. ProfileModeHideSK).
func bindWithVisibility(t *testing.T, service *accountdata.BindingService, platformUserID string, modes ...string) {
	t.Helper()
	ctx := context.Background()
	if _, err := service.Bind(ctx, "qq", platformUserID, "12345678901234"); err != nil {
		t.Fatalf("bind %s: %v", platformUserID, err)
	}
	params := accountdata.ProfileSettingsCommandParams{Platform: "qq", PlatformUserID: platformUserID, Server: "jp", RegionExplicit: true}
	for _, mode := range append([]string{accountdata.ProfileModeShowAll}, modes...) {
		if _, err := accountdata.ExecuteProfileSettingsCommand(ctx, service, mode, params); err != nil {
			t.Fatalf("%s for %s: %v", mode, platformUserID, err)
		}
	}
}

// exposureOutcome is what another user gets for each kind of lookup of an
// @-mentioned account: "" when allowed, else the refusal's message ID.
type exposureOutcome struct {
	profile, arrest, sk string
	uidMasked           bool
}

func lookupAsOtherUser(t *testing.T, service *accountdata.BindingService, platformUserID string) exposureOutcome {
	t.Helper()
	ctx := context.Background()
	app := &renderapp.App{Bindings: service}
	at := userQueryParams{Mode: "at_user", Platform: "qq", PlatformUserID: "visibility-requester", AtUserID: platformUserID}
	outcome := exposureOutcome{}
	refusal := func(err error) string {
		if err == nil {
			return ""
		}
		typed := testutil.RequireUserError(t, err, usererror.CodeForbidden, "")
		return typed.Message.ID
	}
	target, err := resolveGameTarget(ctx, at, "jp", true, app, accountdata.ExposureProfile)
	outcome.profile = refusal(err)
	if err == nil {
		outcome.uidMasked = !target.UIDVisible
	}
	target, err = resolveGameTarget(ctx, at, "jp", true, app, accountdata.ExposureArrest)
	outcome.arrest = refusal(err)
	if err == nil {
		outcome.uidMasked = !target.UIDVisible
	}
	req := rendersk.TrackerRankQuery{Region: "jp", RegionExplicit: true, TargetPlatform: "qq", TargetUserID: platformUserID}
	outcome.sk = refusal(resolveTrackerTargetUser(ctx, app, &req, "qq", "visibility-requester"))
	return outcome
}

// TestVisibilityExposuresAreEnforcedSeparately: each setting only guards its
// own kind of lookup by other users. Hiding the UID masks it but keeps the
// profile, arrest and ranking lookups; hiding one lookup leaves the others.
func TestVisibilityExposuresAreEnforcedSeparately(t *testing.T) {
	service := newHandlerTestBindingService(t)
	cases := []struct {
		user  string
		modes []string
		want  exposureOutcome
	}{
		{"vis-all-shown", nil, exposureOutcome{}},
		{"vis-uid-hidden", []string{accountdata.ProfileModeHideID}, exposureOutcome{uidMasked: true}},
		{"vis-sk-hidden", []string{accountdata.ProfileModeHideSK}, exposureOutcome{sk: "binding.target_hidden_sk"}},
		{"vis-profile-hidden", []string{accountdata.ProfileModeHideInfo}, exposureOutcome{profile: "binding.target_hidden"}},
		{"vis-arrest-hidden", []string{accountdata.ProfileModeHideArrest}, exposureOutcome{arrest: "binding.target_hidden_arrest"}},
		{"vis-all-hidden", []string{accountdata.ProfileModeHideAll}, exposureOutcome{profile: "binding.target_hidden", arrest: "binding.target_hidden_arrest", sk: "binding.target_hidden_sk"}},
		{"vis-only-sk", []string{accountdata.ProfileModeHideAll, accountdata.ProfileModeShowSK}, exposureOutcome{profile: "binding.target_hidden", arrest: "binding.target_hidden_arrest"}},
	}
	for _, tc := range cases {
		t.Run(tc.user, func(t *testing.T) {
			bindWithVisibility(t, service, tc.user, tc.modes...)
			if got := lookupAsOtherUser(t, service, tc.user); got != tc.want {
				t.Fatalf("lookups = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The owner's own lookups ignore every exposure setting; only the UID is
// masked.
func TestVisibilityHiddenAllStillServesTheOwner(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	bindWithVisibility(t, service, "vis-owner", accountdata.ProfileModeHideAll)
	app := &renderapp.App{Bindings: service}
	self := userQueryParams{Mode: "self", Platform: "qq", PlatformUserID: "vis-owner"}
	for _, exposure := range accountdata.Exposures {
		target, err := resolveGameTarget(ctx, self, "jp", true, app, exposure)
		if err != nil || target.UIDVisible {
			t.Fatalf("%s self lookup = %+v, %v", exposure, target, err)
		}
	}
	req := rendersk.TrackerRankQuery{Region: "jp", RegionExplicit: true, TargetPlatform: "qq", TargetUserID: "vis-owner"}
	if err := resolveTrackerTargetUser(ctx, app, &req, "qq", "vis-owner"); err != nil || req.UserID == nil {
		t.Fatalf("own ranking lookup = %+v, %v", req.UserID, err)
	}
}

// The arrest reply masks the UID by the UID setting alone.
func TestArrestTextMasksUIDByUIDSetting(t *testing.T) {
	resp := &sekaiapi.GetAnotherProfileResponse{User: sekaiapi.AnotherUser{UserID: 12345678901234, Name: "JPUser", Rank: 100}}
	if text := formatArrestText(resp, defaultEnabledDiffs(), "", false); strings.Contains(text, "12345678901234") {
		t.Fatalf("hidden UID shown: %q", text)
	}
	if text := formatArrestText(resp, defaultEnabledDiffs(), "", true); !strings.Contains(text, "12345678901234") {
		t.Fatalf("visible UID masked: %q", text)
	}
}

// Every visibility route is registered with its own help document and
// triggers that resolve to it.
func TestVisibilityRoutesResolve(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for trigger, path := range map[string]string{
		"/隐藏id":         "profile/visibility/hide",
		"/显示id":         "profile/visibility/show",
		"/隐藏sk":         "profile/sk/hide",
		"/显示sk":         "profile/sk/show",
		"/隐藏个人信息":       "profile/info/hide",
		"/展示个人信息":       "profile/info/show",
		"/隐藏逮捕":         "profile/arrest/hide",
		"/显示逮捕":         "profile/arrest/show",
		"/隐藏全部":         "profile/visibility/hide-all",
		"/显示全部":         "profile/visibility/show-all",
		"/隐私设置":         "profile/visibility/status",
		"/pjsk hide sk": "profile/sk/hide",
		"/jp隐藏sk":       "profile/sk/hide",
		"/隐藏抓包":         "profile/suite/hide",
		"/逮捕":           "arrest",
		"/个人信息":         "profile",
	} {
		matched := corehandler.MatchCommandHandler(trigger)
		if matched.Handler == nil || matched.Handler.GetPath() != path {
			t.Errorf("%s resolves to %+v, want %s", trigger, matched, path)
		}
	}
}
