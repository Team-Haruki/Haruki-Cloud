package handler

import (
	"context"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/chartstyle"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func resolveHarukiUserChartStyle(ctx context.Context, app *renderapp.App, harukiUserID int) string {
	if app == nil || app.PJSK == nil || harukiUserID <= 0 {
		return ""
	}

	settings, err := accountdata.GetUserSettings(ctx, app.PJSK, harukiUserID)
	if err != nil || settings == nil {
		return ""
	}
	return chartstyle.Normalize(settings.ChartStyle)
}

func resolveRequesterHarukiUserChartStyle(ctx context.Context, app *renderapp.App, platform, platformUserID string) string {
	if app == nil || app.Bindings == nil {
		return ""
	}

	harukiUserID, _, err := app.Bindings.ResolveUserBinding(
		ctx,
		platform,
		platformUserID,
		accountdata.GlobalDefaultBindingScope,
	)
	if harukiUserID <= 0 {
		return ""
	}
	if err != nil && harukiUserID == 0 {
		return ""
	}
	return resolveHarukiUserChartStyle(ctx, app, harukiUserID)
}
