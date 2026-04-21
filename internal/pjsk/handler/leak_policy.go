package handler

import renderregion "haruki-cloud/internal/pjsk/region"

func allowReadOnlyLeaks(region string) bool {
	return renderregion.WithDefault(renderregion.Normalize(region)) != renderregion.JP
}
