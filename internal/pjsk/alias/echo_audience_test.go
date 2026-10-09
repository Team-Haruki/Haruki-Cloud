package alias

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

var noEcho = i18n.RenderOptions{NoEcho: true}

// TestAliasTextFollowsTheAudience: the submitter (or any non-admin) sees
// unreviewed alias text only with parameter echo; replies that pass the
// alias review admin check always show it, because admins review it.
func TestAliasTextFollowsTheAudience(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 74, "Tell Your World")
	deps.addAdmin(t, ctx, "qq", "admin", "Audience Admin")
	const submitted = "AUDIENCE_SENTINEL"

	run := func(mode string, params any) (i18n.Message, error) {
		t.Helper()
		return ExecuteCommand(ctx, deps.service, mode, aliasCommandJSON(t, params))
	}

	// Submitter (not an admin): no text without echo.
	reply, err := run(ModeAdd, AddCommandParams{AliasType: PjskAliasTypeMusic, Platform: "qq", PlatformUserID: "10001", Target: "74", Aliases: []string{submitted}})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got := reply.Render(noEcho); strings.Contains(got, submitted) || !strings.Contains(got, "#1") {
		t.Fatalf("submitter reply without echo = %q", got)
	}
	if got := reply.String(); !strings.Contains(got, submitted) {
		t.Fatalf("submitter reply with echo = %q", got)
	}
	_, err = run(ModeAdd, AddCommandParams{AliasType: PjskAliasTypeMusic, Platform: "qq", PlatformUserID: "10002", Target: "74", Aliases: []string{submitted}})
	typed := testutil.RequireUserError(t, err, usererror.CodeInput, "alias.already_pending")
	if got := typed.Message.Render(noEcho); strings.Contains(got, submitted) {
		t.Fatalf("duplicate submission without echo = %q", got)
	}

	// A non-admin cannot receive the review replies at all.
	for _, mode := range []string{ModePendingList, ModeSubmitter} {
		params := any(ReviewListCommandParams{Platform: "qq", PlatformUserID: "10001"})
		if mode == ModeSubmitter {
			params = SubmitterCommandParams{Platform: "qq", PlatformUserID: "10001", ReviewID: 1}
		}
		if _, err := run(mode, params); err == nil {
			t.Fatalf("%s by a non-admin succeeded", mode)
		}
	}

	// Admin: the text is shown without echo (rejection last, it removes the alias).
	for _, step := range []struct {
		name   string
		mode   string
		params any
	}{
		{"pending list", ModePendingList, ReviewListCommandParams{Platform: "qq", PlatformUserID: "admin"}},
		{"submitter", ModeSubmitter, SubmitterCommandParams{Platform: "qq", PlatformUserID: "admin", ReviewID: 1}},
		{"reject", ModeReject, RejectCommandParams{Platform: "qq", PlatformUserID: "admin", ReviewID: 1, Reason: "test"}},
	} {
		name := step.name
		reply, err := run(step.mode, step.params)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := reply.Render(noEcho); !strings.Contains(got, submitted) {
			t.Fatalf("admin %s without echo hides the alias: %q", name, got)
		}
	}
}

// Errors on the approval path only reach admins, so they name the alias
// without echo too.
func TestApprovalErrorsShowAliasTextToAdmins(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 74, "Tell Your World")
	deps.addAdmin(t, ctx, "qq", "admin", "Approval Admin")
	const submitted = "APPROVAL_SENTINEL"
	if _, err := deps.service.Submit(ctx, PjskAliasTypeMusic, "qq", "10001", "74", []string{submitted}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, err := deps.service.ListPending(ctx, "qq", "admin")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	// Approved meanwhile by another path: approving the pending one clashes.
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 74, submitted)
	_, err = deps.service.Approve(ctx, "qq", "admin", []int64{pending[0].ReviewID})
	typed := testutil.RequireUserError(t, err, usererror.CodeInput, "alias.review.already_approved")
	if got := typed.Message.Render(noEcho); !strings.Contains(got, submitted) {
		t.Fatalf("admin approval error without echo = %q", got)
	}

	// The batch duplicate error hides the typed review IDs but not the alias.
	dup := i18n.M("alias.duplicate_in_batch", i18n.Data{"UserFirst": i18n.UserNumber(987654321), "UserSecond": i18n.UserNumber(987654322), "Kind": i18n.M("alias.kind.music"), "Alias": submitted})
	if got := dup.Render(noEcho); !strings.Contains(got, submitted) || strings.Contains(got, "987654321") {
		t.Fatalf("batch duplicate without echo = %q", got)
	}
	if got := conflictsNameError(PjskAliasTypeMusic, submitted, false).(*usererror.Error).Message.Render(noEcho); strings.Contains(got, submitted) {
		t.Fatalf("submitter name clash without echo = %q", got)
	}
	if got := conflictsNameError(PjskAliasTypeMusic, submitted, true).(*usererror.Error).Message.Render(noEcho); !strings.Contains(got, submitted) {
		t.Fatalf("admin name clash without echo = %q", got)
	}
}
