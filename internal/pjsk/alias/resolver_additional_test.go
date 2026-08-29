package alias

import (
	"context"
	"strings"
	"testing"
)

func TestResolverReadinessEmptyAndDirectMatches(t *testing.T) {
	ctx := context.Background()
	unready := NewService(nil, nil, nil)
	for _, resolve := range []func() (int, bool, error){
		func() (int, bool, error) { return unready.TryResolveMusicID(ctx, "1") },
		func() (int, bool, error) { return unready.TryResolveMusicTitleOrAliasID(ctx, "title") },
		func() (int, bool, error) { return unready.TryResolveCharacterID(ctx, "1") },
	} {
		id, ok, err := resolve()
		if err != nil || ok || id != 0 {
			t.Fatalf("unready resolver = %d, %t, %v", id, ok, err)
		}
	}

	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 101, "Unique Song")
	deps.addCharacter(t, ctx, 11, "初音", "未来", "Hatsune", "Miku")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 101, "独特曲")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 11, "公主殿下")

	for _, input := range []struct {
		name string
		call func() (int, bool, error)
		want int
	}{
		{"music id", func() (int, bool, error) { return deps.service.TryResolveMusicID(ctx, "101") }, 101},
		{"music title", func() (int, bool, error) { return deps.service.TryResolveMusicID(ctx, "Unique Song") }, 101},
		{"music alias", func() (int, bool, error) { return deps.service.TryResolveMusicID(ctx, "独特曲") }, 101},
		{"character id", func() (int, bool, error) { return deps.service.TryResolveCharacterID(ctx, "11") }, 11},
		{"character name", func() (int, bool, error) { return deps.service.TryResolveCharacterID(ctx, "Hatsune Miku") }, 11},
		{"character alias", func() (int, bool, error) { return deps.service.TryResolveCharacterID(ctx, "公主殿下") }, 11},
	} {
		t.Run(input.name, func(t *testing.T) {
			id, ok, err := input.call()
			if err != nil || !ok || id != input.want {
				t.Fatalf("resolver = %d, %t, %v", id, ok, err)
			}
		})
	}

	for _, call := range []func() (int, bool, error){
		func() (int, bool, error) { return deps.service.TryResolveMusicID(ctx, " ") },
		func() (int, bool, error) { return deps.service.TryResolveMusicTitleOrAliasID(ctx, " ") },
		func() (int, bool, error) { return deps.service.TryResolveCharacterID(ctx, " ") },
		func() (int, bool, error) { return deps.service.TryResolveMusicID(ctx, "missing") },
		func() (int, bool, error) { return deps.service.TryResolveCharacterID(ctx, "missing") },
	} {
		id, ok, err := call()
		if err != nil || ok || id != 0 {
			t.Fatalf("missing resolver = %d, %t, %v", id, ok, err)
		}
	}
}

func TestResolverAmbiguityAndMissingIDs(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 201, "Same Song")
	deps.addMusic(t, ctx, 202, "Same Song")
	deps.addCharacter(t, ctx, 21, "天马", "司", "Tenma", "Tsukasa")
	deps.addCharacter(t, ctx, 22, "天马", "司", "Tenma", "Tsukasa")
	for _, id := range []int{201, 202} {
		deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, id, "shared-song")
	}
	for _, id := range []int{21, 22} {
		deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, id, "shared-character")
	}

	for _, call := range []func() error{
		func() error { _, _, err := deps.service.TryResolveMusicID(ctx, "999"); return err },
		func() error { _, _, err := deps.service.TryResolveCharacterID(ctx, "999"); return err },
		func() error { _, _, err := deps.service.TryResolveMusicID(ctx, "Same Song"); return err },
		func() error { _, _, err := deps.service.TryResolveMusicID(ctx, "shared-song"); return err },
		func() error { _, _, err := deps.service.TryResolveMusicTitleOrAliasID(ctx, "shared"); return err },
		func() error { _, _, err := deps.service.TryResolveCharacterID(ctx, "天马司"); return err },
		func() error { _, _, err := deps.service.TryResolveCharacterID(ctx, "shared-character"); return err },
	} {
		if err := call(); err == nil {
			t.Fatal("ambiguous or missing resolver unexpectedly succeeded")
		}
	}

	if _, err := deps.service.resolveEntityByToken(ctx, "unknown", "value"); err == nil {
		t.Fatal("unsupported entity type resolved")
	}
	if _, err := deps.service.resolveMusicByToken(ctx, " "); err == nil {
		t.Fatal("empty music token resolved")
	}
	if _, err := deps.service.resolveCharacterByToken(ctx, " "); err == nil {
		t.Fatal("empty character token resolved")
	}
	if _, err := uniqueMusicFromRows(nil, "title"); err == nil {
		t.Fatal("empty music rows resolved")
	}
}

func TestApprovedAliasViewsDeduplicateAndHandleConflicts(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 301, "Alias Song")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 301, " Beta ")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 301, "alpha")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 301, "ALPHA")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 301, "")

	aliases, err := deps.service.ListApprovedMusicAliases(ctx, 301)
	if err != nil {
		t.Fatalf("list music aliases: %v", err)
	}
	if len(aliases) != 2 || strings.ToLower(strings.TrimSpace(aliases[0])) != "alpha" || strings.ToLower(strings.TrimSpace(aliases[1])) != "beta" {
		t.Fatalf("deduplicated aliases = %#v", aliases)
	}
	if aliases, err := deps.service.ListApprovedMusicAliases(ctx, 0); err != nil || aliases != nil {
		t.Fatalf("invalid music aliases = %#v, %v", aliases, err)
	}
	if aliases, err := NewService(nil, nil, nil).ListApprovedMusicAliases(ctx, 301); err != nil || aliases != nil {
		t.Fatalf("unready music aliases = %#v, %v", aliases, err)
	}

	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 41, "solo")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 42, "conflict")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 43, "CONFLICT")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 0, "invalid-owner")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeCharacter, 44, "")
	aliasMap, err := deps.service.ListApprovedCharacterAliasMap(ctx)
	if err != nil {
		t.Fatalf("list character alias map: %v", err)
	}
	if len(aliasMap) != 1 || aliasMap["solo"] != 41 {
		t.Fatalf("character alias map = %#v", aliasMap)
	}
	if aliasMap, err := NewService(nil, nil, nil).ListApprovedCharacterAliasMap(ctx); err != nil || len(aliasMap) != 0 {
		t.Fatalf("unready character alias map = %#v, %v", aliasMap, err)
	}
}
