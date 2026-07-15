package deck

import (
	"context"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/provider"
)

type contextBindingProvider struct {
	provider.MasterDataProvider
	region renderregion.Value
}

func (p *contextBindingProvider) Region() renderregion.Value {
	return p.region
}

func TestControllerWithContextBindsMasterdataSources(t *testing.T) {
	base := &contextBindingProvider{region: renderregion.JP}
	cardSource := rendercard.NewProviderAdapter(base)
	eventSource := renderevent.NewProviderAdapter(base)
	musicSource := rendermusic.NewProviderAdapter(base)
	controller := NewController(cardSource, eventSource, nil, assets.NewAssetHelper("", nil), nil, renderregion.JP)
	controller.RegisterMusicSource(musicSource)

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "deck")
	clone := controller.WithContext(ctx)

	_, clonedCard, err := clone.resolveCardSource(renderregion.JP)
	if err != nil {
		t.Fatalf("resolveCardSource() error = %v", err)
	}
	if got := clonedCard.(*rendercard.ProviderAdapter).Context().Value(contextKey("request")); got != "deck" {
		t.Fatalf("card source context value = %v", got)
	}

	_, clonedEvent, ok := clone.resolveEventSource(renderregion.JP)
	if !ok {
		t.Fatal("resolveEventSource() did not find JP source")
	}
	if got := clonedEvent.(*renderevent.ProviderAdapter).Context().Value(contextKey("request")); got != "deck" {
		t.Fatalf("event source context value = %v", got)
	}

	_, clonedMusic, ok := clone.resolveMusicSource(renderregion.JP)
	if !ok {
		t.Fatal("resolveMusicSource() did not find JP source")
	}
	if got := clonedMusic.(*rendermusic.ProviderAdapter).Context().Value(contextKey("request")); got != "deck" {
		t.Fatalf("music source context value = %v", got)
	}

	if cardSource.Context().Value(contextKey("request")) != nil ||
		eventSource.Context().Value(contextKey("request")) != nil ||
		musicSource.Context().Value(contextKey("request")) != nil {
		t.Fatal("request context leaked back into shared sources")
	}
}
