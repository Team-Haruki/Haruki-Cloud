package handler

import "testing"

func TestSKTraceRefactorResidualBranches(t *testing.T) {
	ctx := newSKParameterTestContext()
	params := newSKTraceParams(ctx, 9, 21, " Miku ", 100)
	if params["event_id"] != 9 || params["wl_character_id"] != 21 || params["wl_character_query"] != "Miku" || params["compare_rank"] != 100 {
		t.Fatalf("trace metadata params = %#v", params)
	}

	ctx.uidArg = "@not-a-user"
	target, ranks := resolveSKTraceTarget(ctx, "")
	if target.userID != "" || ranks != "" {
		t.Fatalf("invalid mention target = %+v, %q", target, ranks)
	}
	ctx.uidArg = "@123"
	target, ranks = resolveSKTraceTarget(ctx, "10")
	if target.userID != "" || ranks != "10" {
		t.Fatalf("rank-precedence target = %+v, %q", target, ranks)
	}

	params = map[string]any{}
	applySKTraceTarget(params, skTraceTarget{}, "QQ")
	if len(params) != 0 {
		t.Fatalf("empty trace target changed params: %#v", params)
	}
}
