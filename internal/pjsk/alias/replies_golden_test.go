package alias

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAliasRepliesGolden runs the alias review lifecycle and locks the zh-CN
// text of every reply (alias.toml) in testdata/replies.zh-CN.golden, so a
// wording change shows up as a reviewable diff. Regenerate with
// HARUKI_UPDATE_GOLDEN=1 go test ./internal/pjsk/alias/.
func TestAliasRepliesGolden(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 74, "Tell Your World")
	deps.addCharacter(t, ctx, 21, "初音", "ミク", "Hatsune", "Miku")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 74, "tyw")
	deps.addAdmin(t, ctx, "qq", "admin", "Golden Admin")

	var b strings.Builder
	run := func(name, mode string, params any) {
		t.Helper()
		result, err := ExecuteCommand(ctx, deps.service, mode, aliasCommandJSON(t, params))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b.WriteString("== " + name + "\n" + string(result) + "\n")
	}
	admin := ReviewListCommandParams{Platform: "qq", PlatformUserID: "admin"}

	run("query music", ModeQuery, QueryCommandParams{AliasType: PjskAliasTypeMusic, Target: "74"})
	run("query character (none)", ModeQuery, QueryCommandParams{AliasType: PjskAliasTypeCharacter, Target: "21"})
	run("pending (none)", ModePendingList, admin)
	run("add music", ModeAdd, AddCommandParams{AliasType: PjskAliasTypeMusic, Platform: "qq", PlatformUserID: "10001", Target: "74", Aliases: []string{"告诉你的世界", "世界"}})
	run("add character", ModeAdd, AddCommandParams{AliasType: PjskAliasTypeCharacter, Platform: "qq", PlatformUserID: "10001", Target: "21", Aliases: []string{"葱", "公主殿下"}})
	run("pending", ModePendingList, admin)
	pending, err := deps.service.ListPending(ctx, "qq", "admin")
	if err != nil || len(pending) != 4 {
		t.Fatalf("pending = %v, %v", pending, err)
	}
	run("submitter", ModeSubmitter, SubmitterCommandParams{Platform: "qq", PlatformUserID: "admin", ReviewID: pending[0].ReviewID})
	run("approve", ModeApprove, ApproveCommandParams{Platform: "qq", PlatformUserID: "admin", ReviewIDs: []int64{pending[0].ReviewID, pending[2].ReviewID}})
	run("reject", ModeReject, RejectCommandParams{Platform: "qq", PlatformUserID: "admin", ReviewID: pending[1].ReviewID, Reason: "与已有别名重复"})
	run("batch reject", ModeBatchReject, BatchRejectCommandParams{Platform: "qq", PlatformUserID: "admin", ReviewIDs: []int64{pending[3].ReviewID}})
	run("ban submitter", ModeBanSubmitter, BanSubmitterCommandParams{Platform: "qq", PlatformUserID: "admin", TargetPlatform: "qq", TargetPlatformUserID: "10001"})
	run("delete music", ModeDelete, DeleteCommandParams{AliasType: PjskAliasTypeMusic, Platform: "qq", PlatformUserID: "admin", Target: "74", Aliases: []string{"tyw"}})
	run("query music (after review)", ModeQuery, QueryCommandParams{AliasType: PjskAliasTypeMusic, Target: "74"})
	b.WriteString("== fallback name (music)\n" + fallbackEntityName(PjskAliasTypeMusic, 9999) + "\n")
	b.WriteString("== fallback name (character)\n" + fallbackEntityName(PjskAliasTypeCharacter, 99) + "\n")

	path := filepath.Join("testdata", "replies.zh-CN.golden")
	if os.Getenv("HARUKI_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with HARUKI_UPDATE_GOLDEN=1)", path, err)
	}
	if string(want) != b.String() {
		t.Fatalf("%s is out of date; review the diff and regenerate with HARUKI_UPDATE_GOLDEN=1\n--- got ---\n%s", path, b.String())
	}
}
