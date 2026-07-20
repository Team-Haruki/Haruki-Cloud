package sk

import (
	"context"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/provider"
)

type contextBindingProvider struct {
	provider.MasterDataProvider
	region renderregion.Value
}

func (p *contextBindingProvider) Region() renderregion.Value {
	return p.region
}

func TestControllerWithContextBindsEventSources(t *testing.T) {
	base := &contextBindingProvider{region: renderregion.JP}
	eventSource := renderevent.NewProviderAdapter(base)
	controller := NewController(nil)
	controller.SetTrackerIntegration(nil, eventSource, renderassets.NewAssetHelper("", nil))

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "sk")
	clone := controller.WithContext(ctx)
	clonedEvent := clone.eventSourceForRegion("jp")
	if clonedEvent == nil {
		t.Fatal("eventSourceForRegion() did not find JP source")
	}
	if got := clonedEvent.(*renderevent.ProviderAdapter).Context().Value(contextKey("request")); got != "sk" {
		t.Fatalf("event source context value = %v", got)
	}
	if eventSource.Context().Value(contextKey("request")) != nil {
		t.Fatal("request context leaked back into shared event source")
	}
}
