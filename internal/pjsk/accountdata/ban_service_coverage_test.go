package accountdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	usersdb "haruki-cloud/database/users"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/pjsk/parser"

	_ "github.com/mattn/go-sqlite3"
)

func openBanCoverageService(t *testing.T, name string) (*BanService, *usersdb.Client) {
	t.Helper()
	client := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:accountdata_ban_%s_%d?mode=memory&cache=shared&_fk=1", name, time.Now().UnixNano()))
	return NewBanService(client), client
}

func TestBanServiceDefensiveBranches(t *testing.T) {
	ctx := context.Background()
	var nilService *BanService
	nilService.SetAdminQQIDs(nil)
	nilService.SetReadOnly(true)
	if nilService.IsAdmin("qq", "1") || NewBanService(nil) != nil {
		t.Fatal("nil ban service should be defensive")
	}
	if status, err := nilService.GlobalBanStatus(ctx, "qq", "1"); err != nil || status.Active {
		t.Fatalf("nil GlobalBanStatus = %+v, %v", status, err)
	}
	if err := nilService.CheckBan(ctx, "qq", "1", parser.ModuleMusic); err != nil {
		t.Fatalf("nil CheckBan = %v", err)
	}
	if _, err := nilService.Kill(ctx, "1", "reason", nil); err == nil {
		t.Fatal("nil Kill should fail")
	}
	if err := nilService.Back(ctx, "1"); err == nil {
		t.Fatal("nil Back should fail")
	}

	service, _ := openBanCoverageService(t, "hierarchy")
	service.SetAdminQQIDs([]string{"", "-1", "abc", " 00042 ", "42"})
	if !service.IsAdmin("qq", "42") || !service.IsAdmin(" qq ", " 42 ") || service.IsAdmin("discord", "42") {
		t.Fatal("admin roster normalization mismatch")
	}
	if status, err := service.GlobalBanStatus(ctx, "qq", "missing"); err != nil || status.Active {
		t.Fatalf("missing GlobalBanStatus = %+v, %v", status, err)
	}
	if banned, err := service.IsGloballyBanned(ctx, "qq", "missing"); err != nil || banned {
		t.Fatalf("missing IsGloballyBanned = %v, %v", banned, err)
	}
}

func TestBanServiceMutationValidationBranches(t *testing.T) {
	ctx := context.Background()
	service, _ := openBanCoverageService(t, "validation")
	for _, test := range []struct {
		qqID      string
		reason    string
		expiresAt *time.Time
	}{
		{qqID: "", reason: "reason"},
		{qqID: "1", reason: ""},
		{qqID: "1", reason: strings.Repeat("界", 256)},
	} {
		if _, err := service.Kill(ctx, test.qqID, test.reason, test.expiresAt); err == nil {
			t.Fatalf("invalid Kill(%q, len=%d) should fail", test.qqID, len([]rune(test.reason)))
		}
	}
	past := time.Now().Add(-time.Minute)
	if _, err := service.Kill(ctx, "1", "reason", &past); err == nil {
		t.Fatal("past ban expiry should fail")
	}
	service.SetReadOnly(true)
	if _, err := service.Kill(ctx, "1", "reason", nil); err == nil {
		t.Fatal("read-only Kill should fail")
	}
	if err := service.Back(ctx, "1"); err == nil {
		t.Fatal("read-only Back should fail")
	}
	service.SetReadOnly(false)
	if err := service.Back(ctx, " "); err == nil {
		t.Fatal("blank Back should fail")
	}
	if err := service.Back(ctx, "999"); err == nil {
		t.Fatal("Back for a missing user should fail")
	}
}

func TestBanServiceFeatureHierarchyBranches(t *testing.T) {
	ctx := context.Background()
	service, client := openBanCoverageService(t, "feature_hierarchy")
	if _, err := client.User.Create().
		SetID(100).
		SetPlatform("qq").
		SetUserID("100").
		SetPjskMainBanState(true).
		SetPjskMainBanReason("main reason").
		Save(ctx); err != nil {
		t.Fatalf("create feature-banned user: %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleMusic); err == nil || !strings.Contains(err.Error(), "main reason") {
		t.Fatalf("main feature ban = %v", err)
	}
	if _, err := client.User.UpdateOneID(100).
		SetPjskMainBanState(false).
		SetPjskRankingBanState(true).
		SetPjskRankingBanReason("ranking reason").
		Save(ctx); err != nil {
		t.Fatalf("update ranking ban: %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleSK); err == nil || !strings.Contains(err.Error(), "ranking reason") {
		t.Fatalf("ranking feature ban = %v", err)
	}
	if _, err := client.User.UpdateOneID(100).
		SetPjskRankingBanState(false).
		SetPjskAliasBanState(true).
		SetPjskAliasBanReason("alias reason").
		Save(ctx); err != nil {
		t.Fatalf("update alias ban: %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleAlias); err == nil || !strings.Contains(err.Error(), "alias reason") {
		t.Fatalf("alias feature ban = %v", err)
	}
}

func TestBanServiceModuleAndGlobalHierarchyBranches(t *testing.T) {
	ctx := context.Background()
	service, client := openBanCoverageService(t, "module_global_hierarchy")
	if _, err := client.User.Create().
		SetID(100).
		SetPlatform("qq").
		SetUserID("100").
		SetPjskMysekaiBanState(true).
		SetPjskMysekaiBanReason("mysekai reason").
		Save(ctx); err != nil {
		t.Fatalf("create MySekai-banned user: %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleMysekai); err == nil || !strings.Contains(err.Error(), "mysekai reason") {
		t.Fatalf("MySekai feature ban = %v", err)
	}
	if _, err := client.User.UpdateOneID(100).
		SetPjskMysekaiBanState(false).
		SetPjskBanState(true).
		SetPjskBanReason("module reason").
		Save(ctx); err != nil {
		t.Fatalf("update PJSK module ban: %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleMusic); err == nil || !strings.Contains(err.Error(), "module reason") {
		t.Fatalf("PJSK module ban = %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleAdmin); err != nil {
		t.Fatalf("non-PJSK module should ignore PJSK ban: %v", err)
	}
	if _, err := client.User.UpdateOneID(100).
		SetPjskBanState(false).
		SetBanState(true).
		SetBanReason("global reason").
		Save(ctx); err != nil {
		t.Fatalf("update global ban: %v", err)
	}
	if err := service.CheckBan(ctx, "qq", "100", parser.ModuleAdmin); err == nil || !strings.Contains(err.Error(), "global reason") {
		t.Fatalf("global ban = %v", err)
	}
	status, err := service.GlobalBanStatus(ctx, "qq", "100")
	if err != nil || !status.Active || status.Reason != "global reason" {
		t.Fatalf("active GlobalBanStatus = %+v, %v", status, err)
	}
}

func TestGlobalBanStatusHelpers(t *testing.T) {
	if globalBanStatusForUser(nil).Active || globalBanStatusForUser(&usersdb.User{}).Active {
		t.Fatal("nil/inactive user should not have a global ban")
	}
	expired := time.Now().Add(-time.Second)
	if globalBanStatusForUser(&usersdb.User{BanState: true, BanExpiresAt: &expired}).Active {
		t.Fatal("expired global ban should be inactive")
	}
	future := time.Now().Add(time.Hour)
	status := globalBanStatusForUser(&usersdb.User{BanState: true, BanReason: " reason ", BanExpiresAt: &future})
	if !status.Active || status.Reason != "reason" || status.ExpiresAt == nil {
		t.Fatalf("active global ban status = %+v", status)
	}
	if err := globalBanError(GlobalBanStatus{Active: true, Reason: "permanent"}); err == nil || strings.Contains(err.Error(), "封禁至") {
		t.Fatalf("permanent global ban error = %v", err)
	}
	if err := globalBanError(status); err == nil || !strings.Contains(err.Error(), "封禁至") {
		t.Fatalf("timed global ban error = %v", err)
	}
	if err := banError("功能", ""); err == nil || strings.Contains(err.Error(), "原因") {
		t.Fatalf("reason-less ban error = %v", err)
	}
	if err := banError("功能", "reason"); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("reasoned ban error = %v", err)
	}
}

func TestBanModuleMappings(t *testing.T) {
	for _, module := range []parser.TargetModule{
		parser.ModuleCard, parser.ModuleGacha, parser.ModuleMusic, parser.ModuleEvent,
		parser.ModuleDeck, parser.ModuleSK, parser.ModuleMysekai, parser.ModuleProfile,
		parser.ModuleHelp, parser.ModuleEducation, parser.ModuleScore, parser.ModuleStamp,
		parser.ModuleMisc, parser.ModuleArrest, parser.ModuleRegTime, parser.ModuleCheckData,
		parser.ModuleAlias,
	} {
		if !isPJSKModule(module) {
			t.Fatalf("module %v should be treated as PJSK", module)
		}
	}
	if isPJSKModule(parser.ModuleUnknown) || isPJSKModule(parser.ModuleVLive) || isPJSKModule(parser.ModuleAdmin) {
		t.Fatal("non-listed modules should not be treated as PJSK")
	}
	if featureBanFor(parser.ModuleSK) != featureRanking || featureBanFor(parser.ModuleAlias) != featureAlias || featureBanFor(parser.ModuleMysekai) != featureMysekai || featureBanFor(parser.ModuleMusic) != featureMain {
		t.Fatal("feature ban category mapping mismatch")
	}
	if featureNone != 0 {
		t.Fatal("featureNone should remain the zero value")
	}
}

func TestBanServiceDatabaseErrorBranches(t *testing.T) {
	service, client := openBanCoverageService(t, "errors")
	if err := client.Close(); err != nil {
		t.Fatalf("close users DB: %v", err)
	}
	if _, err := service.GlobalBanStatus(context.Background(), "qq", "1"); err == nil {
		t.Fatal("closed DB GlobalBanStatus should fail")
	}
	if err := service.CheckBan(context.Background(), "qq", "1", parser.ModuleMusic); err != nil {
		t.Fatalf("CheckBan intentionally fails open on DB errors: %v", err)
	}
	if err := service.Back(context.Background(), "1"); err == nil {
		t.Fatal("closed DB Back should fail")
	}
	if _, err := service.Kill(context.Background(), "1", "reason", nil); err == nil && !errors.Is(err, context.Canceled) {
		t.Fatal("closed DB Kill should fail")
	}
}
