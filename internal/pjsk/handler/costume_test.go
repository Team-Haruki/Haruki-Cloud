package handler

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
)

func TestCardCostumeCommands(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for _, tt := range []struct {
		command, region string
		card, color     int
	}{
		{"/查服装123", "jp", 123, 0},
		{"/en查服装 123 2", "en", 123, 2},
	} {
		resolved, err := dispatchForTest(context.Background(), Event{Platform: "qq", Message: onebot11.Message{onebot11.Text(tt.command)}, UserId: "12345"})
		if err != nil || resolved == nil {
			t.Fatalf("%s: %v", tt.command, err)
		}
		var query rendercostume.Query
		if err := json.Unmarshal(resolved.Params, &query); err != nil {
			t.Fatal(err)
		}
		if resolved.Module != parser.ModuleCostume || resolved.Mode != "costume-detail" || query.CardID != tt.card || query.ColorPosition != tt.color || query.Region != tt.region {
			t.Fatalf("%s: %+v", tt.command, query)
		}
	}
	for _, args := range []string{"", "miku", "123 mnr", "123 颜色2", "0", "-1", "123 0", "123 -1", "123 2 3"} {
		h := sekaiHandlers{}.CostumeDetailHandle()
		if _, err := h.handleFunc(additionalModuleContext(args, "/查服装", "jp")); err == nil {
			t.Errorf("accepted invalid args %q", args)
		}
	}
}
