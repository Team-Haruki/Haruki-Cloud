package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestMysekaiPhotoHandleBuildsResolvedCommand(t *testing.T) {
	h := sekaiHandlers{}.MysekaiPhotoHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.GetPath() != "mysekai/photo" {
		t.Fatalf("handler path = %q", h.GetPath())
	}

	result, err := h.Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/msp",
		ArgText:    "-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("handler returned %T", result)
	}
	if resolved.Module != parser.ModuleMysekai || resolved.Mode != "mysekai-photo" {
		t.Fatalf("unexpected resolved command: %+v", resolved)
	}

	var params struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Seq != -1 {
		t.Fatalf("params.Seq = %d", params.Seq)
	}
}
