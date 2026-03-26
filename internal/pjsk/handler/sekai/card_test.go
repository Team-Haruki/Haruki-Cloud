package sekai

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestCardImgHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.CardImgHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查卡面",
		ArgText:    "1001",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleCard || resolved.Mode != "card-image" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
	if resolved.Query != "1001" {
		t.Fatalf("resolved.Query = %q", resolved.Query)
	}
}
