package handler

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

func TestResolveDeckRenderProfileAndSnapshotUsesSelectedBinding(t *testing.T) {
	ctx := context.Background()
	service := newBridgeTestBindingServiceWithValidator(t, bridgeMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"jp": {
				"33333333333333": "JP User 1",
				"44444444444444": "JP User 2",
			},
		},
	})

	if _, err := service.Bind(ctx, "qq", "42", "33333333333333"); err != nil {
		t.Fatalf("bind first account: %v", err)
	}
	if _, err := service.Bind(ctx, "qq", "42", "44444444444444"); err != nil {
		t.Fatalf("bind second account: %v", err)
	}

	_, expectedBinding, err := service.ResolveUserBindingBySelector(ctx, "qq", "42", "jp", "u2")
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

	detail, snapshot, err := resolveDeckRenderProfileAndSnapshot(rc, "u2")
	if err != nil {
		t.Fatalf("resolveDeckRenderProfileAndSnapshot() error = %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if detail == nil || detail.Nickname != "selector-snapshot" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if len(provider.selectors) != 1 {
		t.Fatalf("expected one snapshot selector, got %d", len(provider.selectors))
	}

	selector := provider.selectors[0]
	if selector.IMPlatform != "qq" || selector.IMUserID != "42" {
		t.Fatalf("unexpected im selector: %+v", selector)
	}
	if selector.Region != renderregion.JP {
		t.Fatalf("unexpected selector region: %+v", selector.Region)
	}
	if selector.PJSKUserID != expectedBinding.PJSKUserID {
		t.Fatalf("expected selected binding uid %q, got %q", expectedBinding.PJSKUserID, selector.PJSKUserID)
	}
}
