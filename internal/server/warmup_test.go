package server

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/provider"

	"github.com/gofiber/fiber/v3"
)

type warmupTestProvider struct {
	provider.MasterDataProvider
	warmed chan struct{}
	once   sync.Once
}

func (p *warmupTestProvider) WarmMasterdata(context.Context) error {
	p.once.Do(func() { close(p.warmed) })
	return nil
}

func TestStartMasterdataWarmupRunsOnceListening(t *testing.T) {
	startMasterdataWarmup(context.Background(), nil, nil)
	startMasterdataWarmup(context.Background(), fiber.New(), nil)

	warmed := &warmupTestProvider{warmed: make(chan struct{})}
	runtime := &renderapp.App{Providers: map[renderregion.Value]provider.MasterDataProvider{renderregion.JP: warmed}}
	app := fiber.New()
	startMasterdataWarmup(context.Background(), app, runtime)
	select {
	case <-warmed.warmed:
		t.Fatal("warm-up must wait for the listener")
	default:
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	select {
	case <-warmed.warmed:
	case <-time.After(5 * time.Second):
		t.Fatal("warm-up did not run after the listener came up")
	}
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}
