package handler

import (
	"errors"
	"fmt"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/render/education"
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
	err := normalizeEducationAreaError(fmt.Errorf("build: %w", education.ErrAreaItemNotInRegion))
	if _, ok := errors.AsType[onebot11.ReplayError](err); !ok {
		t.Fatalf("expected a replay error, got %v", err)
	}
	other := errors.New("area item masterdata is not available")
	if got := normalizeEducationAreaError(other); got != other {
		t.Fatalf("unrelated error changed: %v", got)
	}
}
