package accountdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	_ "github.com/mattn/go-sqlite3"
)

type accountCoverageIdentity struct {
	id  int
	err error
}

func (r accountCoverageIdentity) ResolveOrCreate(context.Context, string, string) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.id, nil
}

type accountCoverageValidator struct {
	profiles map[string]string
	errors   map[string]error
}

func (v accountCoverageValidator) GetUserProfile(server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if err := v.errors[server]; err != nil {
		return nil, err
	}
	name, ok := v.profiles[server]
	if !ok {
		return nil, sekaiapi.ErrUserNotFound
	}
	return &sekaiapi.GetAnotherProfileResponse{User: sekaiapi.AnotherUser{UserID: 1, Name: name}}, nil
}

func (v accountCoverageValidator) GetUserProfileContext(ctx context.Context, server, userID string) (*sekaiapi.GetAnotherProfileResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return v.GetUserProfile(server, userID)
}

func openAccountCoverageService(t *testing.T, name string, validator ProfileValidator) (*BindingService, *pjskdb.Client) {
	t.Helper()
	client := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:accountdata_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano()))
	return NewBindingService(client, accountCoverageIdentity{id: 42}, validator), client
}

func TestBindingServiceValidationProbeAndLifecycleBranches(t *testing.T) {
	ctx := context.Background()
	validator := accountCoverageValidator{profiles: map[string]string{"jp": "", "tw": "TW Player"}}
	service, client := openAccountCoverageService(t, "lifecycle", validator)

	var nilService *BindingService
	nilService.SetUsersDB(nil)
	nilService.SetFastVerificationProvider(nil)
	nilService.SetProfileBGStorage(nil)
	nilService.SetCensorService(nil)
	nilService.SetReadOnly(true)
	if nilService.IsReady() || service == nil || !service.IsReady() {
		t.Fatal("binding service readiness mismatch")
	}
	if err := nilService.requireWritable(); !errors.Is(err, ErrBindingServiceUnavailable) {
		t.Fatalf("nil requireWritable = %v", err)
	}
	if err := service.requireReady("", "42"); err == nil {
		t.Fatal("blank platform should fail readiness validation")
	}
	if err := service.requireReady("qq", " "); err == nil {
		t.Fatal("blank platform user ID should fail readiness validation")
	}
	if _, err := service.Bind(ctx, "qq", "42", " "); err == nil {
		t.Fatal("blank game UID should fail")
	}
	if _, err := service.Bind(ctx, "qq", "42", "12x"); err == nil {
		t.Fatal("non-numeric game UID should fail")
	}
	service.SetReadOnly(true)
	if _, err := service.Bind(ctx, "qq", "42", "1000"); err == nil {
		t.Fatal("read-only bind should fail")
	}
	service.SetReadOnly(false)

	result, err := service.Bind(ctx, "qq", "42", "1000")
	if err != nil || result.Server != "jp" || result.UserName != "1000" || !result.MultipleServerMatch {
		t.Fatalf("multi-server Bind() = %+v, %v", result, err)
	}
	again, err := service.Bind(ctx, "qq", "42", "1000")
	if err != nil || !again.AlreadyBound || again.SetGlobalDefault || again.SetServerDefault {
		t.Fatalf("repeated Bind() = %+v, %v", again, err)
	}
	items, err := service.List(ctx, "qq", "42")
	if err != nil || len(items) != 1 || items[0].DisplayOrder != 1 {
		t.Fatalf("List() = %+v, %v", items, err)
	}

	failing := accountCoverageValidator{errors: map[string]error{
		"jp": sekaiapi.ErrUserNotFound,
		"cn": sekaiapi.ErrServerMaintenance,
		"tw": errors.New("transport failed"),
		"kr": sekaiapi.ErrUserNotFound,
		"en": sekaiapi.ErrUserNotFound,
	}}
	failingService, _ := openAccountCoverageService(t, "probe_failures", failing)
	if _, err := failingService.Bind(ctx, "qq", "42", "2000"); err == nil || !strings.Contains(err.Error(), "用户不存在") || !strings.Contains(err.Error(), "服务器维护中") || !strings.Contains(err.Error(), "transport failed") {
		t.Fatalf("all-server probe failure = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.probeUID(canceled, "1000"); err == nil {
		t.Fatal("contextual profile probe should honor cancellation")
	}
	identityFailure := NewBindingService(client, accountCoverageIdentity{err: errors.New("identity failed")}, validator)
	if _, err := identityFailure.Bind(ctx, "qq", "42", "1000"); err == nil || !strings.Contains(err.Error(), "identity failed") {
		t.Fatalf("identity failure = %v", err)
	}
}

func TestBindingSelectionResolutionDefaultsAndProperties(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "selection", accountCoverageValidator{profiles: map[string]string{"jp": "JP", "cn": "CN"}})
	if _, err := service.Bind(ctx, "qq", "42", "1001"); err != nil {
		t.Fatalf("bind first: %v", err)
	}
	service.validator = accountCoverageValidator{profiles: map[string]string{"jp": "JP2"}}
	if _, err := service.Bind(ctx, "qq", "42", "1002"); err != nil {
		t.Fatalf("bind second: %v", err)
	}
	service.validator = accountCoverageValidator{profiles: map[string]string{"cn": "CN3"}}
	if _, err := service.Bind(ctx, "qq", "42", "1003"); err != nil {
		t.Fatalf("bind third: %v", err)
	}
	items, err := service.List(ctx, "qq", "42")
	if err != nil || len(items) != 3 {
		t.Fatalf("seeded binding list = %+v, %v", items, err)
	}

	if buildBindingList(nil, nil) != nil || filterBindingsByServer(nil, "") != nil {
		t.Fatal("empty binding helpers should return nil")
	}
	if _, err := selectBinding(nil, "u1", ""); err == nil {
		t.Fatal("selecting from an empty list should fail")
	}
	for _, selector := range []string{"", "u0", "ux", "u999", "missing"} {
		if _, err := selectBinding(items, selector, ""); err == nil {
			t.Fatalf("selectBinding(%q) should fail", selector)
		}
	}
	if _, err := selectBinding(items, "u1", "en"); err == nil {
		t.Fatal("server-scoped selection without bindings should fail")
	}
	if _, err := selectBinding(items, "u9", "jp"); err == nil {
		t.Fatal("out-of-range server selection should fail")
	}
	duplicateUID := []BindingListItem{{BindingID: 1, UserID: "same", Server: "jp"}, {BindingID: 2, UserID: "same", Server: "tw"}}
	if _, err := selectBinding(duplicateUID, "same", ""); err == nil {
		t.Fatal("ambiguous raw UID should fail")
	}
	if got, err := selectBinding(items, items[0].UserID, ""); err != nil || got.BindingID != items[0].BindingID {
		t.Fatalf("raw UID selection = %+v, %v", got, err)
	}
	if got := filterBindingsByServer(items, "JP"); len(got) != 2 {
		t.Fatalf("JP filtered bindings = %+v", got)
	}
	if normalizeSelectorServer("") != "" || normalizeSelectorServer("default") != "" || normalizeSelectorServer("bad") != "bad" || normalizeSelectorServer(" JP ") != "jp" {
		t.Fatal("selector server normalization mismatch")
	}
	if isNumericUID("") || isNumericUID("1a") || !isNumericUID("001") || normalizeUID(" x ") != "x" {
		t.Fatal("UID normalization mismatch")
	}

	if _, err := service.ResolveOwnBindingForUIDQuery(ctx, "qq", "42", "u2", "jp"); err != nil {
		t.Fatalf("ResolveOwnBindingForUIDQuery selector: %v", err)
	}
	if got, err := service.ResolveOwnBindingForUIDQuery(ctx, "qq", "42", "", "jp"); err != nil || got.Server != "jp" {
		t.Fatalf("ResolveOwnBindingForUIDQuery JP default = %+v, %v", got, err)
	}
	if _, err := service.ResolveOwnBindingForUIDQuery(ctx, "qq", "42", "", "en"); err == nil {
		t.Fatal("missing server own binding should fail")
	}
	if got, err := service.ResolveOwnBindingForUIDQuery(ctx, "qq", "42", "", ""); err != nil || !got.IsGlobalDefault {
		t.Fatalf("global own binding = %+v, %v", got, err)
	}
	harukiID, resolved, err := service.ResolveUserBindingBySelector(ctx, "qq", "42", "jp", "u2")
	if err != nil || harukiID != 42 || resolved.Server != "jp" {
		t.Fatalf("ResolveUserBindingBySelector = %d, %+v, %v", harukiID, resolved, err)
	}
	if _, err := service.currentBindingEntityBySelector(ctx, "qq", "42", "jp", "u2"); err != nil {
		t.Fatalf("currentBindingEntityBySelector: %v", err)
	}
	if _, err := service.bindingListItemByID(ctx, "qq", "42", 999); err == nil {
		t.Fatal("missing binding list item should fail")
	}

	if scope, label, err := normalizeDefaultScope("unsupported"); err != nil || scope != "unsupported" || label != "UNSUPPORTED" {
		t.Fatalf("custom default scope normalization = %q, %q, %v", scope, label, err)
	}
	if scope, label, err := normalizeDefaultScope(""); err != nil || scope != GlobalDefaultBindingScope || label != "全局" || defaultScopeType(scope) != DefaultScopeGlobal {
		t.Fatalf("global default normalization = %q, %q, %v", scope, label, err)
	}
	if scope, label, err := normalizeDefaultScope(" CN "); err != nil || scope != "cn" || label != "CN" || defaultScopeType(scope) != DefaultScopeServer {
		t.Fatalf("server default normalization = %q, %q, %v", scope, label, err)
	}
	if _, err := service.SetDefault(ctx, "qq", "42", "u1", "jp", "cn"); err == nil {
		t.Fatal("setting CN default to JP binding should fail")
	}
	if _, err := service.SetDefault(ctx, "qq", "42", "u1", "jp", "bad"); err == nil {
		t.Fatal("invalid default scope should fail")
	}
	if _, err := service.ClearDefault(ctx, "qq", "42", "u2", "jp", "default"); err == nil {
		t.Fatal("clearing global default with the wrong binding should fail")
	}
	cleared, err := service.ClearDefault(ctx, "qq", "42", "", "", "default")
	if err != nil || cleared.Binding.IsGlobalDefault {
		t.Fatalf("clear global default by scope = %+v, %v", cleared, err)
	}
	if _, err := service.ClearDefault(ctx, "qq", "42", "", "", "default"); err == nil {
		t.Fatal("clearing missing global default should fail")
	}
	if _, err := service.SetDefault(ctx, "qq", "42", "u2", "jp", "default"); err != nil {
		t.Fatalf("restore global default: %v", err)
	}
	cleared, err = service.ClearDefault(ctx, "qq", "42", "u2", "jp", "default")
	if err != nil || cleared.Scope != DefaultScopeGlobal {
		t.Fatalf("clear selected global default = %+v, %v", cleared, err)
	}

	if _, err := service.Swap(ctx, "qq", "42", "", "u1", ""); err == nil {
		t.Fatal("swap without both selectors should fail")
	}
	if _, err := service.Swap(ctx, "qq", "42", "u1", "u1", ""); err == nil {
		t.Fatal("swap with identical selectors should fail")
	}
	if _, err := service.Swap(ctx, "qq", "42", "u1", "u9", "jp"); err == nil {
		t.Fatal("swap with invalid selector should fail")
	}
	service.SetReadOnly(true)
	if _, err := service.SetDefault(ctx, "qq", "42", "u1", "jp", "default"); err == nil {
		t.Fatal("read-only SetDefault should fail")
	}
	if _, err := service.ClearDefault(ctx, "qq", "42", "", "", "jp"); err == nil {
		t.Fatal("read-only ClearDefault should fail")
	}
	if _, err := service.Swap(ctx, "qq", "42", "u1", "u2", "jp"); err == nil {
		t.Fatal("read-only Swap should fail")
	}
	service.SetReadOnly(false)

	if got := effectiveBindingDisplayOrder(nil); got != 0 {
		t.Fatalf("nil binding display order = %d", got)
	}
	if got := effectiveBindingDisplayOrder(&pjskdb.UserBinding{ID: 7}); got != 7 {
		t.Fatalf("fallback binding display order = %d", got)
	}
	if got := effectiveBindingListDisplayOrder(BindingListItem{BindingID: 8}); got != 8 {
		t.Fatalf("fallback list display order = %d", got)
	}
	if got := effectiveBindingListDisplayOrder(BindingListItem{}); got != 0 {
		t.Fatalf("zero list display order = %d", got)
	}

	// Make the default resolver miss and exercise the visible-binding fallback.
	if _, err := client.UserDefaultBinding.Delete().Exec(ctx); err != nil {
		t.Fatalf("delete defaults: %v", err)
	}
	first := items[0]
	if _, err := client.UserBinding.UpdateOneID(first.BindingID).SetVisible(true).Save(ctx); err != nil {
		t.Fatalf("make fallback binding visible: %v", err)
	}
	if harukiID, got, err := service.ResolveUserBinding(ctx, "qq", "42", first.Server); err != nil || harukiID != 42 || got.BindingID != first.BindingID {
		t.Fatalf("fallback ResolveUserBinding = %d, %+v, %v", harukiID, got, err)
	}
	if _, _, err := service.ResolveUserBinding(ctx, "qq", "42", "en"); !errors.Is(err, ErrNoBinding) {
		t.Fatalf("missing ResolveUserBinding = %v", err)
	}
}

func TestBindingTransactionHelpersAndBackgroundValueHelpers(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "transactions", accountCoverageValidator{})
	_ = service
	account, err := client.GameAccount.Create().SetServer("jp").SetUserID("9001").Save(ctx)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	b1, err := client.UserBinding.Create().SetHarukiUserID(42).SetGameAccountID(account.ID).SetDisplayOrder(0).Save(ctx)
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	account2, err := client.GameAccount.Create().SetServer("jp").SetUserID("9002").Save(ctx)
	if err != nil {
		t.Fatalf("create second account: %v", err)
	}
	b2, err := client.UserBinding.Create().SetHarukiUserID(42).SetGameAccountID(account2.ID).SetDisplayOrder(0).Save(ctx)
	if err != nil {
		t.Fatalf("create second binding: %v", err)
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if next, err := nextBindingDisplayOrderTx(ctx, tx, 999); err != nil || next != 1 {
		t.Fatalf("next empty display order = %d, %v", next, err)
	}
	if err := ensureBindingDisplayOrdersTx(ctx, tx, 999); err != nil {
		t.Fatalf("normalize empty binding orders: %v", err)
	}
	if err := ensureBindingDisplayOrdersTx(ctx, tx, 42); err != nil {
		t.Fatalf("normalize duplicate binding orders: %v", err)
	}
	if err := ensureBindingDisplayOrdersTx(ctx, tx, 42); err != nil {
		t.Fatalf("already-normalized binding orders: %v", err)
	}
	if created, err := ensureDefaultBindingTx(ctx, tx, 42, "default", b1.ID); err != nil || !created {
		t.Fatalf("create default = %v, %v", created, err)
	}
	if created, err := ensureDefaultBindingTx(ctx, tx, 42, "default", b2.ID); err != nil || created {
		t.Fatalf("existing default = %v, %v", created, err)
	}
	if row, err := upsertDefaultBindingTx(ctx, tx, 42, "default", b2.ID); err != nil || row.BindingID != b2.ID {
		t.Fatalf("update default = %+v, %v", row, err)
	}
	if row, err := upsertDefaultBindingTx(ctx, tx, 42, "jp", b1.ID); err != nil || row.BindingID != b1.ID {
		t.Fatalf("create server default = %+v, %v", row, err)
	}
	if existing, err := getOrCreateGameAccountTx(ctx, tx, " JP ", " 9001 "); err != nil || existing.ID != account.ID {
		t.Fatalf("existing game account = %+v, %v", existing, err)
	}
	if created, err := getOrCreateGameAccountTx(ctx, tx, "tw", "9003"); err != nil || created.UserID != "9003" {
		t.Fatalf("create game account = %+v, %v", created, err)
	}
	if _, err := getOrCreateGameAccountTx(ctx, tx, "", "9004"); err == nil {
		t.Fatal("invalid game account identity should fail")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit helpers tx: %v", err)
	}

	if !hasDefaultScope([]*pjskdb.UserDefaultBinding{{Server: "jp"}}, "jp") || hasDefaultScope(nil, "jp") {
		t.Fatal("default scope lookup mismatch")
	}
	path := " user_upload/profile_bg/jp/test.jpg "
	bg := &drawing.ProfileBgSettings{ImgPath: &path, Blur: 3, Alpha: 60, Vertical: true}
	if cloneProfileBGSettings(nil) != nil || !hasCustomProfileBGImage(bg) || hasCustomProfileBGImage(nil) {
		t.Fatal("profile background clone/image checks mismatch")
	}
	cloned := cloneProfileBGSettings(bg)
	*cloned.ImgPath = "changed"
	if *bg.ImgPath == "changed" {
		t.Fatal("profile background path was not deeply cloned")
	}
	if !sameProfileBGPath(bg, &drawing.ProfileBgSettings{ImgPath: new("user_upload/profile_bg/jp/test.jpg")}) || sameProfileBGPath(bg, nil) {
		t.Fatal("profile background path comparison mismatch")
	}
	uploadedPath := " new.jpg "
	merged := mergeUploadedProfileBGSettings(bg, &drawing.ProfileBgSettings{ImgPath: &uploadedPath, Vertical: false})
	if merged == bg || *merged.ImgPath != "new.jpg" || merged.Blur != 3 || merged.Vertical {
		t.Fatalf("merged profile background = %+v", merged)
	}
	if mergeUploadedProfileBGSettings(nil, bg) == bg || clearProfileBGImagePath(nil) != nil || clearProfileBGImagePath(bg).ImgPath != nil {
		t.Fatal("profile background merge/clear helpers mismatch")
	}
	if bindingGameAccount(nil) != nil || bindingGameAccountID(nil) != 0 || bindingServer(nil) != "" || bindingUserID(nil) != "" || resolveBindingProfileBG(nil) != nil {
		t.Fatal("nil binding helpers should return zero values")
	}
	edgeBinding := &pjskdb.UserBinding{Edges: pjskdb.UserBindingEdges{GameAccount: &pjskdb.GameAccount{ID: account.ID, Server: " JP ", UserID: " 9001 ", Bg: bg}}}
	if bindingGameAccountID(edgeBinding) != account.ID || bindingServer(edgeBinding) != "jp" || bindingUserID(edgeBinding) != "9001" || resolveBindingProfileBG(edgeBinding) == nil {
		t.Fatal("binding game-account helpers mismatch")
	}
	accountID := account.ID
	edgeBinding.GameAccountID = &accountID
	if bindingGameAccountID(edgeBinding) != account.ID {
		t.Fatal("explicit binding game-account ID should win")
	}
	if got, err := loadProfileBackground(ctx, nil, account.ID); err != nil || got != nil {
		t.Fatalf("nil DB loadProfileBackground = %+v, %v", got, err)
	}
	if err := upsertProfileBackground(ctx, nil, account.ID, bg); err != nil || deleteProfileBackground(ctx, nil, account.ID) != nil {
		t.Fatal("nil DB profile background helpers should be no-ops")
	}
	if err := upsertProfileBackground(ctx, client, account.ID, bg); err != nil {
		t.Fatalf("upsert profile background: %v", err)
	}
	if got, err := loadProfileBackground(ctx, client, account.ID); err != nil || got == nil || !hasCustomProfileBGImage(got) {
		t.Fatalf("load profile background = %+v, %v", got, err)
	}
	if err := deleteProfileBackground(ctx, client, account.ID); err != nil {
		t.Fatalf("delete profile background: %v", err)
	}
	if err := deleteProfileBackground(ctx, client, 99999); err != nil {
		t.Fatalf("delete missing profile background should be nil: %v", err)
	}
}
