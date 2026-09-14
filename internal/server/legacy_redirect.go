package server

import (
	"net/url"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/core/urlhost"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/storage"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

const (
	legacyImageCacheRoute = "/ic/*"
	// legacyRedirectCacheControl caches the redirect itself; the objects'
	// Cache-Control is owned by node Caddy (E6).
	legacyRedirectCacheControl = "public, max-age=86400"
)

// registerImageCacheRoute mounts /ic/*. With legacy_redirect.enabled and at
// least one image host it answers 301 to the host URL of the same key (E3,
// keep for at least 30 days after the cutover); otherwise, with
// image_cache.dir set, it serves that directory exactly as before.
func registerImageCacheRoute(app *fiber.App, cfg harukiConfig.ImageCacheConfig, hosts *urlhost.Set, log *harukiLogger.Logger) {
	switch {
	case cfg.LegacyRedirect.Enabled && hosts.Len() > 0:
		app.Get(legacyImageCacheRoute, legacyImageCacheRedirect(hosts))
		log.Info("image cache legacy route redirects", "http_route", legacyImageCacheRoute)
	case cfg.Dir != "":
		if cfg.LegacyRedirect.Enabled {
			log.Warn("image cache legacy redirect needs image_cache.hosts or uri; serving image_cache.dir", "http_route", legacyImageCacheRoute)
		}
		app.Get(legacyImageCacheRoute, static.New(cfg.Dir))
		log.Info("image cache static serving enabled", "http_route", legacyImageCacheRoute)
	}
}

func legacyImageCacheRedirect(hosts *urlhost.Set) fiber.Handler {
	return func(c fiber.Ctx) error {
		raw, err := url.PathUnescape(c.Params("*"))
		if err != nil {
			return fiber.ErrNotFound
		}
		key, err := storage.CleanKey(raw)
		if err != nil {
			return fiber.ErrNotFound
		}
		segments := strings.Split(string(key), "/")
		for i, segment := range segments {
			segments[i] = url.PathEscape(segment)
		}
		target, ok := hosts.URL("", strings.Join(segments, "/"))
		if !ok {
			return fiber.ErrNotFound
		}
		c.Set(fiber.HeaderCacheControl, legacyRedirectCacheControl)
		return c.Redirect().Status(fiber.StatusMovedPermanently).To(target)
	}
}

// renderImageHosts returns the runtime's image host set, nil without a runtime.
func renderImageHosts(runtime *renderapp.App) *urlhost.Set {
	if runtime == nil {
		return nil
	}
	return runtime.ImageHosts
}
