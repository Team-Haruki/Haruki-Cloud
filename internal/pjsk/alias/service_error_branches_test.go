package alias

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAliasServiceUnavailableBranches(t *testing.T) {
	ctx := context.Background()
	var service *Service

	checks := []struct {
		name string
		run  func() error
	}{
		{name: "submit", run: func() error { _, err := service.Submit(ctx, "music", "qq", "1", "1", []string{"alias"}); return err }},
		{name: "query", run: func() error { _, err := service.Query(ctx, "music", "1"); return err }},
		{name: "pending", run: func() error { _, err := service.ListPending(ctx, "qq", "1"); return err }},
		{name: "approve", run: func() error { _, err := service.Approve(ctx, "qq", "1", []int64{1}); return err }},
		{name: "reject", run: func() error { _, err := service.RejectMany(ctx, "qq", "1", []int64{1}, "reason"); return err }},
		{name: "submitter", run: func() error { _, err := service.GetSubmitter(ctx, "qq", "1", 1); return err }},
		{name: "ban", run: func() error { _, err := service.BanSubmitter(ctx, "qq", "1", "qq", "2"); return err }},
		{name: "delete", run: func() error { _, err := service.Delete(ctx, "music", "qq", "1", "1", []string{"alias"}); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil {
				t.Fatal("unavailable service accepted the request")
			}
		})
	}

	if aliases, err := service.ListApprovedMusicAliases(ctx, 1); err != nil || aliases != nil {
		t.Fatalf("unavailable music aliases = %#v, %v", aliases, err)
	}
	if aliases, err := service.ListApprovedCharacterAliasMap(ctx); err != nil || len(aliases) != 0 {
		t.Fatalf("unavailable character aliases = %#v, %v", aliases, err)
	}
}

func TestAliasReviewAndModerationInputValidation(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 7301, "审核分支曲目一")
	deps.addAdmin(t, ctx, "qq", "admin-branches", "Branch Admin")

	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, " ", "user", "7301", []string{"待审核"}); err == nil || !strings.Contains(err.Error(), "身份") {
		t.Fatalf("expected submitter identity error, got %v", err)
	}
	if _, err := deps.service.Approve(ctx, "qq", "admin-branches", nil); err == nil {
		t.Fatal("empty approval IDs were accepted")
	}
	if _, err := deps.service.Approve(ctx, "qq", "admin-branches", []int64{999}); err == nil || !strings.Contains(err.Error(), "999") {
		t.Fatalf("expected missing approval ID error, got %v", err)
	}
	if _, err := deps.service.RejectMany(ctx, "qq", "admin-branches", []int64{1}, " "); err == nil || !strings.Contains(err.Error(), "拒绝原因") {
		t.Fatalf("expected empty rejection reason error, got %v", err)
	}
	if _, err := deps.service.GetSubmitter(ctx, "qq", "admin-branches", 0); err == nil {
		t.Fatal("nonpositive review ID was accepted")
	}
	if _, err := deps.service.GetSubmitter(ctx, "qq", "admin-branches", 999); err == nil || !strings.Contains(err.Error(), "999") {
		t.Fatalf("expected missing submitter ID error, got %v", err)
	}
	if _, err := deps.service.BanSubmitter(ctx, "qq", "admin-branches", " ", "target"); err == nil {
		t.Fatal("empty ban target was accepted")
	}
	for range 2 {
		if _, err := deps.service.BanSubmitter(ctx, "qq", "admin-branches", "qq", "target"); err != nil {
			t.Fatalf("BanSubmitter() error = %v", err)
		}
	}
}

func TestAliasApproveRejectsDuplicateAliasesInBatch(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 7301, "审核分支曲目一")
	deps.addMusic(t, ctx, 7302, "审核分支曲目二")
	deps.addAdmin(t, ctx, "qq", "admin-branches", "Branch Admin")
	pendingIDs := make([]int64, 0, 2)
	for _, entityID := range []int{7301, 7302} {
		row, err := deps.pjsk.PendingAlias.Create().
			SetAliasType(PjskAliasTypeMusic).
			SetAliasTypeID(entityID).
			SetAlias("重复待通过别名").
			SetSubmittedBy("qq:user").
			SetSubmittedAt(time.Now()).
			Save(ctx)
		if err != nil {
			t.Fatalf("create duplicate pending alias: %v", err)
		}
		pendingIDs = append(pendingIDs, row.ID)
	}
	if _, err := deps.service.Approve(ctx, "qq", "admin-branches", pendingIDs); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("expected duplicate batch approval error, got %v", err)
	}
}
