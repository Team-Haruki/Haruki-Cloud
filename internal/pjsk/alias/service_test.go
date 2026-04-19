package alias

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	pjskenttest "haruki-cloud/database/pjsk/enttest"
	"haruki-cloud/database/pjsk/rejectedalias"
	sekaidb "haruki-cloud/database/sekai"
	sekaienttest "haruki-cloud/database/sekai/enttest"
	usersenttest "haruki-cloud/database/users/enttest"
	"haruki-cloud/internal/identity"

	_ "github.com/mattn/go-sqlite3"
)

type aliasTestDeps struct {
	service  *Service
	resolver *identity.Resolver
	pjsk     *pjskdb.Client
	sekai    *sekaidb.Client
}

func newAliasTestDeps(t *testing.T) *aliasTestDeps {
	t.Helper()

	suffix := time.Now().UnixNano()
	pjskClient := pjskenttest.Open(t, "sqlite3", fmt.Sprintf("file:alias_pjsk_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = pjskClient.Close() })
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:alias_sekai_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = sekaiClient.Close() })
	usersClient := usersenttest.Open(t, "sqlite3", fmt.Sprintf("file:alias_users_%d?mode=memory&cache=shared&_fk=1", suffix))
	t.Cleanup(func() { _ = usersClient.Close() })

	resolver := identity.NewResolver(usersClient)
	return &aliasTestDeps{
		service:  NewService(sekaiClient, pjskClient, resolver),
		resolver: resolver,
		pjsk:     pjskClient,
		sekai:    sekaiClient,
	}
}

func (d *aliasTestDeps) addMusic(t *testing.T, ctx context.Context, musicID int, title string) {
	t.Helper()
	_, err := d.sekai.Music.Create().
		SetServerRegion("jp").
		SetGameID(int64(musicID)).
		SetTitle(title).
		Save(ctx)
	if err != nil {
		t.Fatalf("create music %d: %v", musicID, err)
	}
}

func (d *aliasTestDeps) addCharacter(t *testing.T, ctx context.Context, characterID int, firstName, givenName, firstNameEnglish, givenNameEnglish string) {
	t.Helper()
	_, err := d.sekai.Gamecharacter.Create().
		SetServerRegion("jp").
		SetGameID(int64(characterID)).
		SetFirstName(firstName).
		SetGivenName(givenName).
		SetFirstNameEnglish(firstNameEnglish).
		SetGivenNameEnglish(givenNameEnglish).
		Save(ctx)
	if err != nil {
		t.Fatalf("create character %d: %v", characterID, err)
	}
}

func (d *aliasTestDeps) addApprovedAlias(t *testing.T, ctx context.Context, aliasType string, entityID int, aliasText string) {
	t.Helper()
	_, err := d.pjsk.Alias.Create().
		SetAliasType(aliasType).
		SetAliasTypeID(entityID).
		SetAlias(aliasText).
		Save(ctx)
	if err != nil {
		t.Fatalf("create approved alias %q: %v", aliasText, err)
	}
}

func (d *aliasTestDeps) addAdmin(t *testing.T, ctx context.Context, platform, platformUserID, name string) {
	t.Helper()
	harukiUserID, err := d.resolver.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		t.Fatalf("resolve admin identity: %v", err)
	}
	_, err = d.pjsk.AliasAdmin.Create().
		SetHarukiUserID(harukiUserID).
		SetName(name).
		Save(ctx)
	if err != nil {
		t.Fatalf("create alias admin: %v", err)
	}
}

func TestServiceSubmitApproveAndQueryMusic(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 5201, "群青讃歌")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 5201, "群青")
	deps.addAdmin(t, ctx, "qq", "9001", "Alias Admin")

	records, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "42", "群青", []string{"蓝歌", "群青歌"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 submitted aliases, got %d", len(records))
	}
	for i, record := range records {
		if record.ReviewID <= 0 {
			t.Fatalf("record %d has invalid review id: %+v", i, record)
		}
		if record.Entity.AliasType != PjskAliasTypeMusic || record.Entity.ID != 5201 || record.Entity.Name != "群青讃歌" {
			t.Fatalf("record %d resolved wrong entity: %+v", i, record)
		}
	}

	beforeApprove, err := deps.service.Query(ctx, PjskAliasTypeMusic, "5201")
	if err != nil {
		t.Fatalf("Query() before approve error = %v", err)
	}
	if !reflect.DeepEqual(beforeApprove.Aliases, []string{"群青"}) {
		t.Fatalf("unexpected approved aliases before approve: %+v", beforeApprove.Aliases)
	}
	if _, err := deps.service.Query(ctx, PjskAliasTypeMusic, "蓝歌"); err == nil {
		t.Fatal("expected pending alias query to fail before approve")
	}

	pending, err := deps.service.ListPending(ctx, "qq", "9001")
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending aliases, got %d", len(pending))
	}

	approved, err := deps.service.Approve(ctx, "qq", "9001", []int64{records[1].ReviewID, records[0].ReviewID})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if len(approved) != 2 {
		t.Fatalf("expected 2 approved aliases, got %d", len(approved))
	}

	afterApprove, err := deps.service.Query(ctx, PjskAliasTypeMusic, "蓝歌")
	if err != nil {
		t.Fatalf("Query() by approved alias error = %v", err)
	}
	wantAliases := []string{"群青", "群青歌", "蓝歌"}
	sortAliasTexts(wantAliases)
	if !reflect.DeepEqual(afterApprove.Aliases, wantAliases) {
		t.Fatalf("unexpected aliases after approve: got=%+v want=%+v", afterApprove.Aliases, wantAliases)
	}
}

func TestServiceSubmitApproveAndQueryCharacter(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addCharacter(t, ctx, 1, "初音", "未来", "Hatsune", "Miku")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 1, "miku")
	deps.addAdmin(t, ctx, "qq", "9002", "Reviewer B")

	records, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "43", "初音未来", []string{"葱", "公主殿下"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 submitted aliases, got %d", len(records))
	}
	for i, record := range records {
		if record.Entity.AliasType != PjskAliasTypeCharacter || record.Entity.ID != 1 || record.Entity.Name != "初音未来" {
			t.Fatalf("record %d resolved wrong entity: %+v", i, record)
		}
	}

	if _, err := deps.service.Query(ctx, PjskAliasTypeCharacter, "葱"); err == nil {
		t.Fatal("expected pending character alias query to fail before approve")
	}

	pending, err := deps.service.ListPending(ctx, "qq", "9002")
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending aliases, got %d", len(pending))
	}
	for _, record := range pending {
		if record.Entity.AliasType != PjskAliasTypeCharacter {
			t.Fatalf("expected character pending record, got %+v", record)
		}
	}

	if _, err := deps.service.Approve(ctx, "qq", "9002", []int64{records[0].ReviewID, records[1].ReviewID}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	result, err := deps.service.Query(ctx, PjskAliasTypeCharacter, "葱")
	if err != nil {
		t.Fatalf("Query() by approved character alias error = %v", err)
	}
	wantAliases := []string{"miku", "公主殿下", "葱"}
	sortAliasTexts(wantAliases)
	if result.Entity.AliasType != PjskAliasTypeCharacter || result.Entity.ID != 1 || result.Entity.Name != "初音未来" {
		t.Fatalf("unexpected entity: %+v", result.Entity)
	}
	if !reflect.DeepEqual(result.Aliases, wantAliases) {
		t.Fatalf("unexpected aliases after approve: got=%+v want=%+v", result.Aliases, wantAliases)
	}
}

func TestServiceListPendingIncludesMultipleAliasTypes(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 5207, "needLe")
	deps.addCharacter(t, ctx, 2, "天马", "司", "Tenma", "Tsukasa")
	deps.addAdmin(t, ctx, "qq", "9007", "Reviewer C")

	musicRecords, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "44", "5207", []string{"need"})
	if err != nil {
		t.Fatalf("music Submit() error = %v", err)
	}
	characterRecords, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "45", "2", []string{"司君"})
	if err != nil {
		t.Fatalf("character Submit() error = %v", err)
	}

	pending, err := deps.service.ListPending(ctx, "qq", "9007")
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending aliases, got %d", len(pending))
	}
	if pending[0].ReviewID != musicRecords[0].ReviewID || pending[0].Entity.AliasType != PjskAliasTypeMusic {
		t.Fatalf("unexpected first pending record: %+v", pending[0])
	}
	if pending[1].ReviewID != characterRecords[0].ReviewID || pending[1].Entity.AliasType != PjskAliasTypeCharacter {
		t.Fatalf("unexpected second pending record: %+v", pending[1])
	}
}

func TestServiceRejectMovesAliasToRejectedTable(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 5202, "星空轨迹")
	deps.addAdmin(t, ctx, "qq", "9003", "Reviewer A")

	records, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "77", "5202", []string{"夜曲"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	rejected, err := deps.service.Reject(ctx, "qq", "9003", records[0].ReviewID, "与现有命名规则冲突")
	if err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if rejected.ReviewID != records[0].ReviewID || rejected.Alias != "夜曲" {
		t.Fatalf("unexpected rejected record: %+v", rejected)
	}

	row, err := deps.pjsk.RejectedAlias.Query().
		Where(rejectedalias.AliasEqualFold("夜曲")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query rejected alias: %v", err)
	}
	if row.ReviewedBy != "Reviewer A" || row.Reason != "与现有命名规则冲突" {
		t.Fatalf("unexpected rejected alias row: %+v", row)
	}

	result, err := deps.service.Query(ctx, PjskAliasTypeMusic, "5202")
	if err != nil {
		t.Fatalf("Query() after reject error = %v", err)
	}
	if len(result.Aliases) != 0 {
		t.Fatalf("expected no approved aliases after reject, got %+v", result.Aliases)
	}
}

func TestServiceDeleteRemovesApprovedAliases(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 5206, "群青交响")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 5206, "蓝歌")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 5206, "群青歌")
	deps.addAdmin(t, ctx, "qq", "9006", "Delete Admin")

	deleted, err := deps.service.Delete(ctx, PjskAliasTypeMusic, "qq", "9006", "5206", []string{"蓝歌"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(deleted) != 1 || deleted[0].Alias != "蓝歌" {
		t.Fatalf("unexpected deleted aliases: %+v", deleted)
	}

	result, err := deps.service.Query(ctx, PjskAliasTypeMusic, "5206")
	if err != nil {
		t.Fatalf("Query() after delete error = %v", err)
	}
	if !reflect.DeepEqual(result.Aliases, []string{"群青歌"}) {
		t.Fatalf("unexpected aliases after delete: %+v", result.Aliases)
	}
	if _, err := deps.service.Query(ctx, PjskAliasTypeMusic, "蓝歌"); err == nil {
		t.Fatal("expected deleted alias to become unqueryable")
	}
}

func TestTryResolveMusicTitleOrAliasIDSupportsPartialApprovedAlias(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 186, "初音天地開闢神話")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 186, "开天辟地")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 186, "初音开天辟地神话")

	id, ok, err := deps.service.TryResolveMusicTitleOrAliasID(ctx, "开天")
	if err != nil {
		t.Fatalf("TryResolveMusicTitleOrAliasID() error = %v", err)
	}
	if !ok || id != 186 {
		t.Fatalf("unexpected partial alias result: ok=%v id=%d", ok, id)
	}
}

func TestServiceDeleteRemovesApprovedCharacterAliases(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addCharacter(t, ctx, 3, "东云", "绘名", "Shinonome", "Ena")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 3, "Ena")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 3, "えななん")
	deps.addAdmin(t, ctx, "qq", "9008", "Delete Admin")

	deleted, err := deps.service.Delete(ctx, PjskAliasTypeCharacter, "qq", "9008", "3", []string{"Ena"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(deleted) != 1 || deleted[0].Alias != "Ena" {
		t.Fatalf("unexpected deleted aliases: %+v", deleted)
	}

	result, err := deps.service.Query(ctx, PjskAliasTypeCharacter, "3")
	if err != nil {
		t.Fatalf("Query() after delete error = %v", err)
	}
	if !reflect.DeepEqual(result.Aliases, []string{"えななん"}) {
		t.Fatalf("unexpected aliases after delete: %+v", result.Aliases)
	}
}

func TestServiceReviewRequiresAdmin(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 5203, "梦想航线")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 5203, "梦航旧称")
	records, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "88", "5203", []string{"梦航"})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if _, err := deps.service.ListPending(ctx, "qq", "not-admin"); err == nil || !strings.Contains(err.Error(), "你不是别名审核管理员") {
		t.Fatalf("expected non-admin list error, got %v", err)
	}
	if _, err := deps.service.Approve(ctx, "qq", "not-admin", []int64{records[0].ReviewID}); err == nil || !strings.Contains(err.Error(), "你不是别名审核管理员") {
		t.Fatalf("expected non-admin approve error, got %v", err)
	}
	if _, err := deps.service.Reject(ctx, "qq", "not-admin", records[0].ReviewID, "no"); err == nil || !strings.Contains(err.Error(), "你不是别名审核管理员") {
		t.Fatalf("expected non-admin reject error, got %v", err)
	}
	if _, err := deps.service.Delete(ctx, PjskAliasTypeMusic, "qq", "not-admin", "5203", []string{"梦航旧称"}); err == nil || !strings.Contains(err.Error(), "你不是别名审核管理员") {
		t.Fatalf("expected non-admin delete error, got %v", err)
	}
}

func TestServiceSubmitRejectsMusicConflicts(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addMusic(t, ctx, 5204, "群青讃歌")
	deps.addMusic(t, ctx, 5205, "天ノ弱")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 5204, "蓝歌")

	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "66", "5204", []string{"重复歌", " 重复歌 "}); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("expected duplicate alias submission error, got %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "66", "5204", []string{"天ノ弱"}); err == nil || !strings.Contains(err.Error(), "与已有曲名重复") {
		t.Fatalf("expected title conflict error, got %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "66", "5204", []string{"蓝歌"}); err == nil || !strings.Contains(err.Error(), "已审核列表") {
		t.Fatalf("expected approved alias conflict error, got %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "66", "5204", []string{"待审核歌"}); err != nil {
		t.Fatalf("Submit() for pending conflict setup error = %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "66", "5204", []string{"待审核歌"}); err == nil || !strings.Contains(err.Error(), "待审核列表") {
		t.Fatalf("expected pending alias conflict error, got %v", err)
	}
}

func TestServiceSubmitRejectsCharacterConflicts(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)

	deps.addCharacter(t, ctx, 4, "天马", "司", "Tenma", "Tsukasa")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 4, "司君")

	if _, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "67", "4", []string{"葱", " 葱 "}); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("expected duplicate alias submission error, got %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "67", "4", []string{"天马司"}); err == nil || !strings.Contains(err.Error(), "与已有角色名重复") {
		t.Fatalf("expected character name conflict error, got %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "67", "4", []string{"司君"}); err == nil || !strings.Contains(err.Error(), "已审核列表") {
		t.Fatalf("expected approved alias conflict error, got %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "67", "4", []string{"王子殿下"}); err != nil {
		t.Fatalf("Submit() for pending conflict setup error = %v", err)
	}
	if _, err := deps.service.Submit(ctx, PjskAliasTypeCharacter, "qq", "67", "4", []string{"王子殿下"}); err == nil || !strings.Contains(err.Error(), "待审核列表") {
		t.Fatalf("expected pending alias conflict error, got %v", err)
	}
}
