package handler

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestVLiveHandleBuildsCommandRequest(t *testing.T) {
	h := sekaiHandlers{}.LiveHandle()
	h.Regions = []renderregion.Value{renderregion.JP}

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/vlive",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if h.GetPath() != "vlive" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleVLive || resolved.Mode != "vlive-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
}
