package drawing

import "context"

// headerRenderForce asks Drawing to bypass its own result caches for this
// render. It travels inside the artifact directive; Drawing ignores it in
// bytes mode.
const headerRenderForce = "X-Haruki-Render-Force"

// forceRenderFlightSuffix keeps a forced render out of an unforced flight for
// the same key: joining it could hand back the very entry being replaced.
const forceRenderFlightSuffix = "\x00force"

type forceRenderKey struct{}

// WithForceRender marks every render made under ctx as forced: the render
// cache lookup is skipped, Drawing is told to bypass its caches, and the fresh
// result is written back under the usual key so later requests reuse it.
func WithForceRender(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, forceRenderKey{}, true)
}

// ForceRenderFrom reports whether ctx carries WithForceRender.
func ForceRenderFrom(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	forced, _ := ctx.Value(forceRenderKey{}).(bool)
	return forced
}

func forceRenderFlightKey(ctx context.Context, key string) string {
	if ForceRenderFrom(ctx) {
		return key + forceRenderFlightSuffix
	}
	return key
}
