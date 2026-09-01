package misc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
)

func TestBuildAliasListRequestValidation(t *testing.T) {
	controller := NewController(nil)
	tests := []struct {
		name string
		req  drawing.AliasListRequest
	}{
		{name: "title", req: drawing.AliasListRequest{}},
		{name: "entity label", req: drawing.AliasListRequest{Title: "Aliases"}},
		{name: "entity id", req: drawing.AliasListRequest{Title: "Aliases", EntityLabel: "Music"}},
		{name: "entity name", req: drawing.AliasListRequest{Title: "Aliases", EntityLabel: "Music", EntityID: 1}},
		{name: "aliases", req: drawing.AliasListRequest{Title: "Aliases", EntityLabel: "Music", EntityID: 1, EntityName: "Song"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := controller.BuildAliasListRequest(test.req); err == nil {
				t.Fatal("BuildAliasListRequest() error = nil")
			}
		})
	}
}

func TestRenderAliasListValidationAndSuccess(t *testing.T) {
	req := drawing.AliasListRequest{
		Title:       "Aliases",
		EntityLabel: "Music",
		EntityID:    1,
		EntityName:  "Song",
		Aliases:     []string{"alias"},
	}
	validated, err := NewController(nil).BuildAliasListRequest(req)
	if err != nil || validated.EntityID != req.EntityID {
		t.Fatalf("BuildAliasListRequest() = (%+v, %v)", validated, err)
	}
	if _, err := (*Controller)(nil).RenderAliasList(req); err == nil {
		t.Fatal("nil controller RenderAliasList() error = nil")
	}
	if _, err := NewController(nil).RenderAliasList(req); err == nil {
		t.Fatal("controller without drawing client RenderAliasList() error = nil")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("alias-image"))
	}))
	defer server.Close()
	controller := NewController(drawing.NewHarukiDrawingClient(server.URL)).WithContext(context.Background())
	if _, err := controller.RenderAliasList(drawing.AliasListRequest{}); err == nil {
		t.Fatal("invalid RenderAliasList() error = nil")
	}
	data, err := controller.RenderAliasList(req)
	if err != nil || string(data) != "alias-image" {
		t.Fatalf("RenderAliasList() = (%q, %v)", data, err)
	}
}
