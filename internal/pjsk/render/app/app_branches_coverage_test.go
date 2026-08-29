package app

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/provider"
)

func TestProviderForRegionFallbackBranches(t *testing.T) {
	if (*App)(nil).ProviderForRegion(renderregion.JP) != nil {
		t.Fatal("nil app returned a provider")
	}
	jp := provider.NewLocalProvider(t.TempDir(), renderregion.JP)
	en := provider.NewLocalProvider(t.TempDir(), renderregion.EN)

	app := &App{
		Config: Config{DefaultRegion: renderregion.JP},
		Providers: map[renderregion.Value]provider.MasterDataProvider{
			renderregion.JP: jp,
			renderregion.EN: en,
		},
	}
	if got := app.ProviderForRegion(renderregion.EN); got != en {
		t.Fatalf("exact region returned %T", got)
	}
	if got := app.ProviderForRegion(renderregion.Unknown); got != jp {
		t.Fatalf("default region returned %T", got)
	}
	if got := app.ProviderForRegion(renderregion.CN); got != jp {
		t.Fatalf("missing region should use configured provider, got %T", got)
	}

	app.Config.DefaultRegion = renderregion.Unknown
	delete(app.Providers, renderregion.JP)
	if got := app.ProviderForRegion(renderregion.CN); got != en {
		t.Fatalf("map fallback returned %T", got)
	}
	app.Providers[renderregion.EN] = nil
	if got := app.ProviderForRegion(renderregion.CN); got != nil {
		t.Fatalf("nil-only map returned %T", got)
	}

	app.Providers = nil
	app.Provider = jp
	if got := app.ProviderForRegion(renderregion.Unknown); got != jp {
		t.Fatalf("unknown single-provider lookup returned %T", got)
	}
	if got := app.ProviderForRegion(renderregion.JP); got != jp {
		t.Fatalf("matching single-provider lookup returned %T", got)
	}
	if got := app.ProviderForRegion(renderregion.EN); got != nil {
		t.Fatalf("mismatching single-provider lookup returned %T", got)
	}
	app.Provider = nil
	if got := app.ProviderForRegion(renderregion.JP); got != nil {
		t.Fatalf("provider-less app returned %T", got)
	}
}

func TestLocalMasterdataRefreshGuardBranches(t *testing.T) {
	var nilApp *App
	nilApp.startLocalMasterdataRefresh(context.Background(), "root", time.Second)
	(&App{}).startLocalMasterdataRefresh(context.Background(), "root", time.Second)

	local := provider.NewLocalProvider(t.TempDir(), renderregion.JP)
	app := &App{Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.JP: local}}
	app.startLocalMasterdataRefresh(context.Background(), " ", time.Second)
	app.startLocalMasterdataRefresh(context.Background(), "root", -time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.startLocalMasterdataRefresh(ctx, t.TempDir(), time.Millisecond)
	app.startLocalMasterdataRefresh(context.Background(), t.TempDir(), -time.Second)

	var nilState *localMasterdataRefreshState
	nilState.captureInitial()
	nilState.refresh()
	state := newLocalMasterdataRefreshState(t.TempDir(), nil, nil)
	if len(state.providers) != 0 || len(state.signatures) != 0 {
		t.Fatalf("unexpected empty refresh state: %+v", state)
	}
	state.captureInitial()
	state.refresh()
}

func TestResolveMetaLoaderBranches(t *testing.T) {
	configured := meta.NewLoader(nil)
	if got := resolveMetaLoader(context.Background(), configured, 0, ""); got != configured {
		t.Fatal("configured loader was replaced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := resolveMetaLoader(ctx, nil, 0, t.TempDir()); got == nil {
		t.Fatal("default loader was not constructed")
	}
}
