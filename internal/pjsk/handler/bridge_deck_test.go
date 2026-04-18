package handler

import (
	"context"
	"encoding/json"
	"testing"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestResolveDeckRenderProfileAndSnapshotUsesSelectedBinding(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"cn": {
				"11111111111111": "CN User 1",
			},
			"jp": {
				"33333333333333": "JP User 1",
			},
		},
	})

	if _, err := service.Bind(ctx, "qq", "42", "11111111111111"); err != nil {
		t.Fatalf("bind first account: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "33333333333333"); err != nil {
		t.Fatalf("bind second account: %v", err)
	}

	_, expectedBinding, err := service.ResolveUserBindingBySelector(ctx, "qq", "42", "", "u1")
	if err != nil {
		t.Fatalf("resolve selector binding: %v", err)
	}

	provider := &runtimeSnapshotProviderStub{
		snapshot: &runtimeSnapshotStub{
			detail: &drawing.DetailedProfileCardRequest{Nickname: "selector-snapshot"},
		},
	}
	rc := NewRequestContext(ctx, &parser.ResolvedCommand{
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Config: renderapp.Config{
			UserSnapshot: renderapp.UserSnapshotConfig{AllowFallback: true},
		},
		Bindings:  service,
		Snapshots: provider,
	})

	detail, snapshot, region, err := resolveDeckRenderProfileAndSnapshot(rc, "u1")
	if err != nil {
		t.Fatalf("resolveDeckRenderProfileAndSnapshot() error = %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if detail == nil || detail.Nickname != "selector-snapshot" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if region != "cn" {
		t.Fatalf("expected resolved region cn, got %q", region)
	}
	if len(provider.selectors) != 1 {
		t.Fatalf("expected one snapshot selector, got %d", len(provider.selectors))
	}

	selector := provider.selectors[0]
	if selector.IMPlatform != "qq" || selector.IMUserID != "42" {
		t.Fatalf("unexpected im selector: %+v", selector)
	}
	if selector.Region != renderregion.CN {
		t.Fatalf("unexpected selector region: %+v", selector.Region)
	}
	if selector.PJSKUserID != expectedBinding.PJSKUserID {
		t.Fatalf("expected selected binding uid %q, got %q", expectedBinding.PJSKUserID, selector.PJSKUserID)
	}
}

func TestExecuteDeckMySekaiBlocksCNRegion(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	params, err := json.Marshal(struct {
		Deck  map[string]any `json:"deck"`
		Query map[string]any `json:"query"`
	}{
		Deck:  map[string]any{},
		Query: map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	message, err := executeDeck(NewRequestContext(context.Background(), &parser.ResolvedCommand{
		Module:            parser.ModuleDeck,
		Mode:              "deck-mysekai",
		Region:            "cn",
		Params:            params,
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{}))
	if err != nil {
		t.Fatalf("executeDeck() error = %v", err)
	}
	assertSingleMySekaiUnavailableMessage(t, message)
}

func assertSingleMySekaiUnavailableMessage(t *testing.T, message onebot11.Message) {
	t.Helper()
	if len(message) != 1 || message[0].Type != onebot11.TypeText {
		t.Fatalf("unexpected message: %+v", message)
	}
	data, ok := message[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected message data: %+v", message[0].Data)
	}
	if data.Text != "MySekai 功能在此区服暂未开放" {
		t.Fatalf("unexpected text: %q", data.Text)
	}
}
