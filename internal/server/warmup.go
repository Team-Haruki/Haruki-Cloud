package server

import (
	"context"

	renderapp "haruki-cloud/internal/pjsk/render/app"

	"github.com/gofiber/fiber/v3"
)

// startMasterdataWarmup preloads the render runtime's bulk master indexes
// once the HTTP listener is up, so the first card commands after a restart
// do not pay for the cold loads. The warm-up runs in the background with
// bounded concurrency; readiness never waits for it.
func startMasterdataWarmup(ctx context.Context, app *fiber.App, runtime *renderapp.App) {
	if app == nil || runtime == nil {
		return
	}
	app.Hooks().OnListen(func(fiber.ListenData) error {
		runtime.WarmMasterdataIndexes(ctx)
		return nil
	})
}
