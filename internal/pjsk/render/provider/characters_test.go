package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

func TestLocalCharacterProviderGetColorCodeUsesGameCharacterID(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "gameCharacterUnits.json", `[
		{"id":105,"gameCharacterId":5,"unit":"idol","colorCode":"#ABCDEF"}
	]`)

	provider := &localCharacterProvider{store: newLocalStore(root)}
	got, ok := provider.GetColorCode(context.Background(), 5)
	if !ok || got != "#ABCDEF" {
		t.Fatalf("expected color by gameCharacterId, got %q ok=%v", got, ok)
	}
}

func TestDBCharacterProviderGetColorCodeUsesGameCharacterID(t *testing.T) {
	ctx := context.Background()
	client := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:provider_character_color_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	if _, err := client.Gamecharacterunit.Create().
		SetGameID(105).
		SetGameCharacterID(5).
		SetUnit("idol").
		SetColorCode("#ABCDEF").
		SetServerRegion(renderregion.JP.String()).
		Save(ctx); err != nil {
		t.Fatalf("create game character unit: %v", err)
	}

	provider := &dbCharacterProvider{client: client, region: renderregion.JP}
	got, ok := provider.GetColorCode(ctx, 5)
	if !ok || got != "#ABCDEF" {
		t.Fatalf("expected color by gameCharacterId, got %q ok=%v", got, ok)
	}
}
