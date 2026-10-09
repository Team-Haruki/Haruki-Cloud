package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestBuildEducationAreaQueryAllCharacterAliases(t *testing.T) {
	for _, args := range []string{"大树", "大樹 full", "想いの大樹", "全角色"} {
		area, err := buildEducationAreaQuery(args, "/区域道具")
		if err != nil || !area.AllCharacter || area.Tree || area.Flower || area.CharacterQuery != "" {
			t.Fatalf("buildEducationAreaQuery(%q) = %+v, %v", args, area, err)
		}
	}
	// "树" alone still means the tree items only.
	area, err := buildEducationAreaQuery("树", "/区域道具")
	if err != nil || area.AllCharacter || !area.Tree {
		t.Fatalf("tree query = %+v, %v", area, err)
	}
}

func TestNormalizeEducationAreaError(t *testing.T) {
	ctx := context.Background()
	err := normalizeEducationAreaError(ctx, fmt.Errorf("build: %w", education.ErrAreaItemNotInRegion))
	testutil.RequireUserError(t, err, usererror.CodeNotFound, "education.area.big_tree_unavailable")
	other := errors.New("area item masterdata is not available")
	if got := normalizeEducationAreaError(ctx, other); got != other {
		t.Fatalf("unrelated error changed: %v", got)
	}
}

func TestNormalizeEducationAreaErrorNotReleased(t *testing.T) {
	// 想いの大樹's JP upgrade shop opens at 2026-09-30T06:00:00Z.
	ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	err := normalizeEducationAreaError(ctx, &education.AreaItemNotReleasedError{OpensAtMs: 1790748000000})
	reply := testutil.RequireUserError(t, err, usererror.CodeNotFound, "education.area.opens_at")
	if !strings.Contains(reply.Error(), "2026-09-30 15:00") {
		t.Fatalf("unexpected reply: %v", err)
	}

	err = normalizeEducationAreaError(ctx, &education.AreaItemNotReleasedError{})
	testutil.RequireUserError(t, err, usererror.CodeNotFound, "education.area.not_on_sale")
}
