package handler

import (
	"context"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/displaytime"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func resolveHarukiUserTimeZone(ctx context.Context, app *renderapp.App, harukiUserID int) string {
	if app == nil || app.PJSK == nil || harukiUserID <= 0 {
		return displaytime.DefaultTimeZone
	}

	settings, err := accountdata.GetUserSettings(ctx, app.PJSK, harukiUserID)
	if err != nil || settings == nil {
		return displaytime.DefaultTimeZone
	}
	return displaytime.NormalizeTimeZone(settings.TimeZone)
}

func resolveRequesterHarukiUserTimeZone(ctx context.Context, app *renderapp.App, platform, platformUserID string) string {
	if app == nil || app.Bindings == nil {
		return displaytime.DefaultTimeZone
	}

	harukiUserID, _, err := app.Bindings.ResolveUserBinding(
		ctx,
		platform,
		platformUserID,
		accountdata.GlobalDefaultBindingScope,
	)
	if harukiUserID <= 0 {
		return displaytime.DefaultTimeZone
	}
	if err != nil && harukiUserID == 0 {
		return displaytime.DefaultTimeZone
	}
	return resolveHarukiUserTimeZone(ctx, app, harukiUserID)
}
