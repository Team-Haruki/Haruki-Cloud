package handler

import (
	"context"
	"strings"
	"testing"

	aliases "haruki-cloud/internal/pjsk/alias"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestAliasParsingEdgeBranches(t *testing.T) {
	if aliasTypeLabel("other") != "目标" || aliasQueryTokenPrompt("other") != "ID 或 名称 或 已审核别名" {
		t.Fatal("unexpected fallback alias labels")
	}

	bulkCases := []string{"", "\nignored", "target\n \n\t"}
	for _, input := range bulkCases {
		if _, _, err := parseEntityAliasBulkArgs(input, "usage"); err == nil {
			t.Fatalf("parseEntityAliasBulkArgs(%q) unexpectedly succeeded", input)
		}
	}
	target, values, err := parseEntityAliasBulkArgs("target\r\n\r\n first \r\n second", "usage")
	if err != nil || target != "target" || len(values) != 2 {
		t.Fatalf("bulk parse = %q, %#v, %v", target, values, err)
	}

	for _, input := range []string{"", "1 nope", "0"} {
		if _, err := parseAliasReviewIDsWithUsage(input, "usage"); err == nil {
			t.Fatalf("parseAliasReviewIDsWithUsage(%q) unexpectedly succeeded", input)
		}
	}
	for _, input := range []string{"", "1 2", "-1"} {
		if _, err := parseAliasReviewID(input, "usage"); err == nil {
			t.Fatalf("parseAliasReviewID(%q) unexpectedly succeeded", input)
		}
	}

	targetCases := []struct {
		args, platform string
		at             []string
	}{
		{"", "qq", nil},
		{"qq:", "qq", nil},
		{"123", "", nil},
	}
	for _, tc := range targetCases {
		if _, _, err := parseAliasSubmissionTarget(tc.args, tc.platform, tc.at, "usage"); err == nil {
			t.Fatalf("parseAliasSubmissionTarget(%q, %q) unexpectedly succeeded", tc.args, tc.platform)
		}
	}
	if platform, userID, err := parseAliasSubmissionTarget("123", " qq ", []string{"  ", "ignored"}, "usage"); err != nil || platform != "qq" || userID != "123" {
		t.Fatalf("plain submission target = %q, %q, %v", platform, userID, err)
	}

	for _, input := range []string{"", "1", "nope reason", "0 reason"} {
		if _, _, err := parseAliasRejectArgs(input); err == nil {
			t.Fatalf("parseAliasRejectArgs(%q) unexpectedly succeeded", input)
		}
	}
	if id, reason, err := parseAliasRejectArgs("7   useful reason"); err != nil || id != 7 || reason != "useful reason" {
		t.Fatalf("reject parse = %d, %q, %v", id, reason, err)
	}
}

func TestAliasHandlersRejectInvalidArguments(t *testing.T) {
	ctx := func(args string) HarrukiSekaiHandlerContext {
		return HarrukiSekaiHandlerContext{PjskHandlerContext: PjskHandlerContext{
			Context: context.Background(), Platform: "qq", UserId: "actor", ArgText: args,
		}}
	}
	handlers := []struct {
		handler HarukiSekaiCommandHandler
		args    string
	}{
		{sekaiHandlers{}.MusicAliasQueryHandle(), ""},
		{sekaiHandlers{}.MusicAliasAddHandle(), ""},
		{sekaiHandlers{}.MusicAliasDeleteHandle(), ""},
		{sekaiHandlers{}.CharacterAliasQueryHandle(), ""},
		{sekaiHandlers{}.CharacterAliasAddHandle(), ""},
		{sekaiHandlers{}.CharacterAliasDeleteHandle(), ""},
		{sekaiHandlers{}.AliasPendingHandle(), "unexpected"},
		{sekaiHandlers{}.AliasSubmitterHandle(), ""},
		{sekaiHandlers{}.AliasBanSubmitterHandle(), ""},
		{sekaiHandlers{}.AliasApproveHandle(), ""},
		{sekaiHandlers{}.AliasRejectHandle(), ""},
		{sekaiHandlers{}.AliasBatchRejectHandle(), ""},
	}
	for _, tc := range handlers {
		if _, err := tc.handler.handleFunc(ctx(tc.args)); err == nil {
			t.Fatalf("handler %s unexpectedly accepted %q", tc.handler.Path, tc.args)
		}
	}
}

func TestAliasExecutionAndImageGuardBranches(t *testing.T) {
	if _, err := executeAlias(&RequestContext{}); err == nil || !strings.Contains(err.Error(), "别名服务未就绪") {
		t.Fatalf("executeAlias unavailable error = %v", err)
	}
	if message, ok, err := tryRenderAliasQueryAsImage(nil); message != nil || ok || err != nil {
		t.Fatalf("nil image attempt = %#v, %v, %v", message, ok, err)
	}
	if _, ok, err := tryRenderAliasQueryAsImage(&RequestContext{App: &renderapp.App{}}); ok || err != nil {
		t.Fatalf("unconfigured image attempt = %v, %v", ok, err)
	}
	if buildCharacterAliasTrimPath(0) != nil {
		t.Fatal("non-positive character ID should not produce a trim path")
	}
	if _, ok := buildAliasListImageRequest(nil, aliases.PjskAliasTypeMusic, nil, "UTC"); ok {
		t.Fatal("nil alias result should not produce an image request")
	}
	if _, ok := buildAliasListImageRequest(nil, "other", &aliases.QueryResult{}, "UTC"); ok {
		t.Fatal("unsupported alias type should not produce an image request")
	}
}
