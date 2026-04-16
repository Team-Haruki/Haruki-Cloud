package sekai

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestVLiveHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.LiveHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/vlive",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if h.GetPath() != "vlive" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleVLive || resolved.Mode != "vlive-list" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}
}
