package handler

import (
	"context"
	"testing"

	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestResolveGameTargetSelectorUsesGlobalIndicesWithoutExplicitRegion(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"cn": {
				"11111111111111": "CN User 1",
			},
			"jp": {
				"33333333333333": "JP User 1",
				"44444444444444": "JP User 2",
			},
		},
	})

	for _, uid := range []string{"11111111111111", "33333333333333", "44444444444444"} {
		if _, err := service.Bind(ctx, "qq", "42", uid); err != nil {
			t.Fatalf("bind %s: %v", uid, err)
		}
	}

	target, err := resolveGameTarget(ctx, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "u2",
	}, "jp", false, &renderapp.App{Bindings: service})
	if err != nil {
		t.Fatalf("resolveGameTarget() error = %v", err)
	}
	if target.PJSKUserID != "33333333333333" {
		t.Fatalf("expected global u2 to resolve jp first account, got %q", target.PJSKUserID)
	}
}

func TestResolveGameTargetSelectorUsesServerScopedIndicesWithExplicitRegion(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"cn": {
				"11111111111111": "CN User 1",
			},
			"jp": {
				"33333333333333": "JP User 1",
				"44444444444444": "JP User 2",
			},
		},
	})

	for _, uid := range []string{"11111111111111", "33333333333333", "44444444444444"} {
		if _, err := service.Bind(ctx, "qq", "42", uid); err != nil {
			t.Fatalf("bind %s: %v", uid, err)
		}
	}

	target, err := resolveGameTarget(ctx, userQueryParams{
		Mode:           "self",
		Platform:       "qq",
		PlatformUserID: "42",
		Selector:       "u2",
	}, "jp", true, &renderapp.App{Bindings: service})
	if err != nil {
		t.Fatalf("resolveGameTarget() error = %v", err)
	}
	if target.PJSKUserID != "44444444444444" {
		t.Fatalf("expected jp-scoped u2 to resolve jp second account, got %q", target.PJSKUserID)
	}
}

func TestResolveBindingWithFallbackSelectorUsesGlobalIndicesWithoutExplicitRegion(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingServiceWithValidator(t, handlerMultiRegionBindingValidator{
		profiles: map[string]map[string]string{
			"cn": {
				"11111111111111": "CN User 1",
			},
			"jp": {
				"33333333333333": "JP User 1",
				"44444444444444": "JP User 2",
			},
		},
	})

	for _, uid := range []string{"11111111111111", "33333333333333", "44444444444444"} {
		if _, err := service.Bind(ctx, "qq", "42", uid); err != nil {
			t.Fatalf("bind %s: %v", uid, err)
		}
	}

	_, binding, err := resolveBindingWithFallback(
		ctx,
		service,
		"qq",
		"42",
		"jp",
		false,
		bindingResolutionOptions{Selector: "u2"},
	)
	if err != nil {
		t.Fatalf("resolveBindingWithFallback() error = %v", err)
	}
	if binding == nil || binding.PJSKUserID != "33333333333333" {
		t.Fatalf("expected global u2 to resolve jp first account, got %+v", binding)
	}
}
