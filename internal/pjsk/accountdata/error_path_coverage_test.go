package accountdata

import (
	"context"
	"errors"
	"strings"
	"testing"

	pjskdb "haruki-cloud/database/pjsk"
	pjskschema "haruki-cloud/ent/pjsk/schema"
	"haruki-cloud/internal/pjsk/drawing"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func TestUserSettingsValidationNilRowsAndDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := GetUserSettings(ctx, nil, 1); err == nil {
		t.Fatal("nil DB GetUserSettings should fail")
	}
	if _, err := GetUserSettings(ctx, &pjskdb.Client{}, 0); err == nil {
		t.Fatal("zero user GetUserSettings should fail")
	}
	if UpsertUserSettings(ctx, nil, 1, &pjskschema.UserSettings{}) == nil || UpsertUserSettings(ctx, &pjskdb.Client{}, 0, &pjskschema.UserSettings{}) == nil {
		t.Fatal("invalid UpsertUserSettings should fail")
	}
	if _, err := IncrNoncompliantBGCount(ctx, nil, 1); err == nil {
		t.Fatal("nil DB counter increment should fail")
	}
	if _, err := IncrNoncompliantBGCount(ctx, &pjskdb.Client{}, 0); err == nil {
		t.Fatal("zero user counter increment should fail")
	}
	service, client := openAccountCoverageService(t, "user_settings_edges", accountCoverageValidator{})
	if err := UpsertUserSettings(ctx, client, 42, nil); err != nil {
		t.Fatalf("nil settings upsert should be a no-op: %v", err)
	}
	if _, err := client.UserPreference.Create().SetHarukiUserID(42).Save(ctx); err != nil {
		t.Fatalf("create nil-settings preference: %v", err)
	}
	settings, err := GetUserSettings(ctx, client, 42)
	if err != nil || settings == nil || settings.NoncompliantBGCount != 0 {
		t.Fatalf("nil-settings row = %+v, %v", settings, err)
	}
	var nilService *BindingService
	if _, _, err := nilService.GetUserSettings(ctx, "qq", "42"); !errors.Is(err, ErrBindingServiceUnavailable) {
		t.Fatalf("nil service GetUserSettings = %v", err)
	}
	if _, err := nilService.UpsertUserSettings(ctx, "qq", "42", settings); !errors.Is(err, ErrBindingServiceUnavailable) {
		t.Fatalf("nil service UpsertUserSettings = %v", err)
	}
	identityFailure := NewBindingService(client, accountCoverageIdentity{err: errors.New("identity failed")}, accountCoverageValidator{})
	if _, _, err := identityFailure.GetUserSettings(ctx, "qq", "42"); err == nil || !strings.Contains(err.Error(), "identity failed") {
		t.Fatalf("identity GetUserSettings failure = %v", err)
	}
	if _, err := identityFailure.UpsertUserSettings(ctx, "qq", "42", settings); err == nil || !strings.Contains(err.Error(), "identity failed") {
		t.Fatalf("identity UpsertUserSettings failure = %v", err)
	}
	service.SetReadOnly(true)
	if _, err := service.UpsertUserSettings(ctx, "qq", "42", settings); err == nil {
		t.Fatal("read-only service settings upsert should fail")
	}
	service.SetReadOnly(false)
	if err := client.Close(); err != nil {
		t.Fatalf("close settings DB: %v", err)
	}
	if _, err := GetUserSettings(ctx, client, 42); err == nil {
		t.Fatal("closed DB GetUserSettings should fail")
	}
	if err := UpsertUserSettings(ctx, client, 42, settings); err == nil {
		t.Fatal("closed DB UpsertUserSettings should fail")
	}
	if _, err := IncrNoncompliantBGCount(ctx, client, 42); err == nil {
		t.Fatal("closed DB counter increment should fail")
	}
}

func TestBindingQueryInterceptorStableErrorBranches(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "query_errors", accountCoverageValidator{})
	forced := errors.New("forced query failure")
	client.Intercept(pjskdb.InterceptFunc(func(pjskdb.Querier) pjskdb.Querier {
		return pjskdb.QuerierFunc(func(context.Context, pjskdb.Query) (pjskdb.Value, error) {
			return nil, forced
		})
	}))
	binding := &pjskdb.UserBinding{
		ID:           1,
		HarukiUserID: 42,
		Verified:     true,
		Edges: pjskdb.UserBindingEdges{GameAccount: &pjskdb.GameAccount{
			ID: 1, Server: "jp", UserID: "9001",
		}},
	}
	service.bgStorage = accountCoverageBGStorage{}
	for name, call := range map[string]func() error{
		"list":             func() error { _, err := service.List(ctx, "qq", "42"); return err },
		"unbind":           func() error { _, err := service.Unbind(ctx, "qq", "42", "u1", ""); return err },
		"set default":      func() error { _, err := service.SetDefault(ctx, "qq", "42", "u1", "", "default"); return err },
		"clear default":    func() error { _, err := service.ClearDefault(ctx, "qq", "42", "", "", "default"); return err },
		"swap":             func() error { _, err := service.Swap(ctx, "qq", "42", "u1", "u2", ""); return err },
		"resolve":          func() error { _, _, err := service.ResolveUserBinding(ctx, "qq", "42", "jp"); return err },
		"verified list":    func() error { _, err := service.ListVerifiedBindings(ctx, "qq", "42", "jp"); return err },
		"set background":   func() error { _, err := service.setBindingProfileBG(ctx, "qq", "42", binding, "url"); return err },
		"clear background": func() error { _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); return err },
		"adjust background": func() error {
			_, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, new(1), nil, nil)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Fatalf("%s should return the forced query failure", name)
		}
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("start intercepted transaction: %v", err)
	}
	defer tx.Rollback()
	if _, err := nextBindingDisplayOrderTx(ctx, tx, 42); err == nil {
		t.Fatal("intercepted next display order should fail")
	}
	if err := ensureBindingDisplayOrdersTx(ctx, tx, 42); err == nil {
		t.Fatal("intercepted display-order normalization should fail")
	}
}

func TestBindingMutationHookStableErrorBranches(t *testing.T) {
	ctx := context.Background()
	service, client := openAccountCoverageService(t, "mutation_errors", accountCoverageValidator{profiles: map[string]string{"jp": "First"}})
	if _, err := service.Bind(ctx, "qq", "42", "9101"); err != nil {
		t.Fatalf("bind first mutation account: %v", err)
	}
	service.validator = accountCoverageValidator{profiles: map[string]string{"jp": "Second"}}
	if _, err := service.Bind(ctx, "qq", "42", "9102"); err != nil {
		t.Fatalf("bind second mutation account: %v", err)
	}
	items, err := service.List(ctx, "qq", "42")
	if err != nil || len(items) != 2 {
		t.Fatalf("mutation binding list = %+v, %v", items, err)
	}
	binding, err := service.currentBindingEntityBySelector(ctx, "qq", "42", "jp", "u1")
	if err != nil {
		t.Fatalf("load mutation binding: %v", err)
	}
	path := DefaultProfileBGRelativeDir + "/jp/existing.jpg"
	if _, err := client.UserBinding.UpdateOneID(binding.ID).SetVerified(true).Save(ctx); err != nil {
		t.Fatalf("verify mutation binding: %v", err)
	}
	if _, err := client.GameAccount.UpdateOneID(bindingGameAccountID(binding)).SetBg(&drawing.ProfileBgSettings{ImgPath: &path, Blur: 2, Alpha: 60}).Save(ctx); err != nil {
		t.Fatalf("seed mutation background: %v", err)
	}
	binding.Verified = true
	forced := errors.New("forced mutation failure")
	client.Use(func(pjskdb.Mutator) pjskdb.Mutator {
		return pjskdb.MutateFunc(func(context.Context, pjskdb.Mutation) (pjskdb.Value, error) {
			return nil, forced
		})
	})
	service.bgStorage = accountCoverageBGStorage{}
	service.fastVerifier = accountCoverageFastVerifier{records: []sekaiapi.UserGameBinding{{Server: "jp", GameUserID: "9101"}}}

	for name, call := range map[string]func() error{
		"visible":          func() error { _, err := service.SetBindingVisible(ctx, "qq", "42", "jp", true); return err },
		"suite visible":    func() error { _, err := service.SetBindingSuiteVisible(ctx, "qq", "42", "jp", true); return err },
		"mysekai visible":  func() error { _, err := service.SetBindingMySekaiVisible(ctx, "qq", "42", "jp", true); return err },
		"set default":      func() error { _, err := service.SetDefault(ctx, "qq", "42", "u2", "jp", "default"); return err },
		"clear default":    func() error { _, err := service.ClearDefault(ctx, "qq", "42", "", "", "default"); return err },
		"swap":             func() error { _, err := service.Swap(ctx, "qq", "42", "u1", "u2", "jp"); return err },
		"unbind":           func() error { _, err := service.Unbind(ctx, "qq", "42", "u1", "jp"); return err },
		"set background":   func() error { _, err := service.setBindingProfileBG(ctx, "qq", "42", binding, "url"); return err },
		"clear background": func() error { _, err := service.clearBindingProfileBG(ctx, "qq", "42", binding); return err },
		"adjust background": func() error {
			_, err := service.adjustBindingProfileBG(ctx, "qq", "42", binding, new(3), nil, nil)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Fatalf("%s should return the forced mutation failure", name)
		}
	}
	binding.Verified = false
	if _, _, err := service.verifyBindingEntity(ctx, "qq", "42", binding); err == nil {
		t.Fatal("verification update should return the forced mutation failure")
	}
	service.bgStorage = accountCoverageBGStorage{saveErr: errors.New("save failed")}
	binding.Verified = true
	if _, err := service.setBindingProfileBG(ctx, "qq", "42", binding, "url"); err == nil || !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("profile background save failure = %v", err)
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("start mutation transaction: %v", err)
	}
	defer tx.Rollback()
	if _, err := ensureDefaultBindingTx(ctx, tx, 99, "default", items[0].BindingID); err == nil {
		t.Fatal("default creation should return forced mutation failure")
	}
	if _, err := upsertDefaultBindingTx(ctx, tx, 99, "jp", items[0].BindingID); err == nil {
		t.Fatal("default upsert creation should return forced mutation failure")
	}
}

func TestBindingHelpersRemainingStableBranches(t *testing.T) {
	ctx := context.Background()
	service, _ := openAccountCoverageService(t, "helper_remaining", accountCoverageValidator{})

	if _, err := service.ClearDefault(ctx, "", "42", "", "", "default"); err == nil {
		t.Fatal("blank identity should fail clear-by-scope readiness")
	}
	if _, err := service.SetDefault(ctx, "", "42", "u1", "", "default"); err == nil {
		t.Fatal("blank identity should fail default readiness")
	}
	service.SetReadOnly(true)
	if _, err := service.updateDefault(ctx, "qq", "42", "u1", "", "default", false); err == nil {
		t.Fatal("private default update should honor read-only mode")
	}
	service.SetReadOnly(false)

	identityFailure := NewBindingService(service.pjskDB, accountCoverageIdentity{err: errors.New("identity failed")}, accountCoverageValidator{})
	if _, err := identityFailure.ClearDefault(ctx, "qq", "42", "", "", "default"); err == nil {
		t.Fatal("clear-by-scope should propagate identity failure")
	}
	if _, err := identityFailure.SetDefault(ctx, "qq", "42", "u1", "", "default"); err == nil {
		t.Fatal("default update should propagate identity failure")
	}
	if _, _, err := identityFailure.ResolveUserBinding(ctx, "qq", "42", "jp"); err == nil {
		t.Fatal("binding resolution should propagate identity failure")
	}
	if _, _, err := identityFailure.ResolveUserBindingBySelector(ctx, "qq", "42", "jp", "u1"); err == nil {
		t.Fatal("selector resolution should propagate identity failure")
	}

	if _, err := service.ResolveOwnBindingForUIDQuery(ctx, "", "42", "", ""); err == nil {
		t.Fatal("own binding resolution should validate identity")
	}
	if _, err := service.currentBindingEntity(ctx, "", "42", "jp"); err == nil {
		t.Fatal("current binding lookup should validate identity")
	}
	if _, err := service.currentBindingEntityBySelector(ctx, "", "42", "jp", "u1"); err == nil {
		t.Fatal("current selector lookup should validate identity")
	}
	if _, _, err := service.ResolveUserBindingBySelector(ctx, "", "42", "jp", "u1"); err == nil {
		t.Fatal("selector resolution should validate identity")
	}
	if _, err := service.ResolveOwnBindingForUIDQuery(ctx, "qq", "42", "", ""); err == nil {
		t.Fatal("empty binding list should not resolve an own binding")
	}
	if _, err := service.currentBindingEntity(ctx, "qq", "42", "jp"); !errors.Is(err, ErrNoBinding) {
		t.Fatalf("empty current binding lookup = %v", err)
	}
	if _, _, err := service.ResolveUserBindingBySelector(ctx, "qq", "42", "jp", "u1"); err == nil {
		t.Fatal("empty selector resolution should fail")
	}
	if _, err := service.currentBindingEntityBySelector(ctx, "qq", "42", "jp", "u1"); err == nil {
		t.Fatal("empty current selector lookup should fail")
	}

	queryService, queryClient := openAccountCoverageService(t, "helper_query_remaining", accountCoverageValidator{})
	forced := errors.New("forced helper query failure")
	queryClient.Intercept(pjskdb.InterceptFunc(func(pjskdb.Querier) pjskdb.Querier {
		return pjskdb.QuerierFunc(func(context.Context, pjskdb.Query) (pjskdb.Value, error) {
			return nil, forced
		})
	}))
	if _, err := queryService.ResolveOwnBindingForUIDQuery(ctx, "qq", "42", "", ""); !errors.Is(err, forced) {
		t.Fatalf("own binding query failure = %v", err)
	}
	if _, err := queryService.bindingListItemByID(ctx, "qq", "42", 1); !errors.Is(err, forced) {
		t.Fatalf("binding-list item query failure = %v", err)
	}
	if _, _, err := queryService.ResolveUserBindingBySelector(ctx, "qq", "42", "jp", "u1"); !errors.Is(err, forced) {
		t.Fatalf("selector list query failure = %v", err)
	}

	makeBinding := func(id int, server string) *pjskdb.UserBinding {
		return &pjskdb.UserBinding{
			ID:           id,
			DisplayOrder: 1,
			Edges: pjskdb.UserBindingEdges{
				GameAccount: &pjskdb.GameAccount{ID: id, Server: server, UserID: string(rune('0' + id))},
			},
		}
	}
	for name, bindings := range map[string][]*pjskdb.UserBinding{
		"server less":    {makeBinding(1, "tw"), makeBinding(2, "jp")},
		"server greater": {makeBinding(1, "jp"), makeBinding(2, "tw")},
		"binding id":     {makeBinding(2, "jp"), makeBinding(1, "jp")},
	} {
		if got := buildBindingList(bindings, nil); len(got) != 2 {
			t.Fatalf("%s sorted binding count = %d", name, len(got))
		}
	}
	if got := effectiveBindingDisplayOrder(&pjskdb.UserBinding{}); got != 0 {
		t.Fatalf("zero binding display order = %d", got)
	}
}

func TestEnsureBindingDisplayOrdersDuplicatePositiveValues(t *testing.T) {
	ctx := context.Background()
	_, client := openAccountCoverageService(t, "duplicate_positive_order", accountCoverageValidator{})
	for i, userID := range []string{"9201", "9202"} {
		account, err := client.GameAccount.Create().SetServer("jp").SetUserID(userID).Save(ctx)
		if err != nil {
			t.Fatalf("create account %d: %v", i, err)
		}
		if _, err := client.UserBinding.Create().SetHarukiUserID(42).SetGameAccountID(account.ID).SetDisplayOrder(1).Save(ctx); err != nil {
			t.Fatalf("create duplicate-order binding %d: %v", i, err)
		}
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("start duplicate-order transaction: %v", err)
	}
	if err := ensureBindingDisplayOrdersTx(ctx, tx, 42); err != nil {
		_ = tx.Rollback()
		t.Fatalf("normalize positive duplicate orders: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit normalized duplicate orders: %v", err)
	}
	rows, err := client.UserBinding.Query().All(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("normalized bindings = %+v, %v", rows, err)
	}
	orders := map[int]bool{}
	for _, row := range rows {
		orders[row.DisplayOrder] = true
	}
	if !orders[1] || !orders[2] {
		t.Fatalf("normalized display orders = %+v", orders)
	}
}
