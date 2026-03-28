package pjsk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"haruki-cloud/api"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"github.com/gofiber/fiber/v3"
)

const (
	renderDispatchBuild  = "build"
	renderDispatchRender = "render"
)

type RenderDispatchRequest struct {
	Target    string          `json:"target"`
	Operation string          `json:"operation"`
	Payload   json.RawMessage `json:"payload"`
}

type renderDispatchTarget struct {
	buildPath  string
	renderPath string
}

type renderDispatchRegistry map[string]renderDispatchTarget

type RenderDispatchHandler struct {
	registry renderDispatchRegistry
}

func registerRenderDispatchRoute(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil {
		return
	}

	handler := &RenderDispatchHandler{
		registry: newRenderDispatchRegistry(runtime),
	}
	router.Post("/render", handler.Dispatch)
}

func newRenderDispatchRegistry(runtime *renderapp.App) renderDispatchRegistry {
	registry := make(renderDispatchRegistry)
	registry.add(runtime != nil && runtime.Cards != nil, "card/detail", "/internal/pjsk/card/detail")
	registry.add(runtime != nil && runtime.Cards != nil, "card/list", "/internal/pjsk/card/list")
	registry.add(runtime != nil && runtime.Cards != nil, "card/box", "/internal/pjsk/card/box")
	registry.add(runtime != nil && runtime.Decks != nil, "deck/recommend", "/internal/pjsk/deck/recommend")
	registry.add(runtime != nil && runtime.Decks != nil, "deck/recommend/auto", "/internal/pjsk/deck/recommend/auto")
	registry.add(runtime != nil && runtime.Events != nil, "event/detail", "/internal/pjsk/event/detail")
	registry.add(runtime != nil && runtime.Events != nil, "event/list", "/internal/pjsk/event/list")
	registry.add(runtime != nil && runtime.Events != nil, "event/record", "/internal/pjsk/event/record")
	registry.add(runtime != nil && runtime.Gachas != nil, "gacha/detail", "/internal/pjsk/gacha/detail")
	registry.add(runtime != nil && runtime.Gachas != nil, "gacha/list", "/internal/pjsk/gacha/list")
	registry.add(runtime != nil && runtime.Honors != nil, "honor", "/internal/pjsk/honor")
	registry.add(runtime != nil && runtime.Profiles != nil, "profile", "/internal/pjsk/profile")
	registry.add(runtime != nil && runtime.Misc != nil, "misc/chara-birthday", "/internal/pjsk/misc/chara-birthday")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/resource", "/internal/pjsk/mysekai/resource")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/map", "/internal/pjsk/mysekai/map")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/fixture-list", "/internal/pjsk/mysekai/fixture-list")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/fixture-detail", "/internal/pjsk/mysekai/fixture-detail")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/door-upgrade", "/internal/pjsk/mysekai/door-upgrade")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/music-record", "/internal/pjsk/mysekai/music-record")
	registry.add(runtime != nil && runtime.MySekai != nil, "mysekai/talk-list", "/internal/pjsk/mysekai/talk-list")
	registry.add(runtime != nil && runtime.Music != nil, "music/detail", "/internal/pjsk/music/detail")
	registry.add(runtime != nil && runtime.Music != nil, "music/brief-list", "/internal/pjsk/music/brief-list")
	registry.add(runtime != nil && runtime.Music != nil, "music/list", "/internal/pjsk/music/list")
	registry.add(runtime != nil && runtime.Music != nil, "music/progress", "/internal/pjsk/music/progress")
	registry.add(runtime != nil && runtime.Music != nil, "music/rewards/detail", "/internal/pjsk/music/rewards/detail")
	registry.add(runtime != nil && runtime.Music != nil, "music/rewards/basic", "/internal/pjsk/music/rewards/basic")
	registry.add(runtime != nil && runtime.Music != nil, "music/chart", "/internal/pjsk/music/chart")
	registry.add(runtime != nil && runtime.Edu != nil, "education/challenge-live", "/internal/pjsk/education/challenge-live")
	registry.add(runtime != nil && runtime.Edu != nil, "education/power-bonus", "/internal/pjsk/education/power-bonus")
	registry.add(runtime != nil && runtime.Edu != nil, "education/area-item", "/internal/pjsk/education/area-item")
	registry.add(runtime != nil && runtime.Edu != nil, "education/bonds", "/internal/pjsk/education/bonds")
	registry.add(runtime != nil && runtime.Edu != nil, "education/leader-count", "/internal/pjsk/education/leader-count")
	registry.add(runtime != nil && runtime.SK != nil, "sk/line", "/internal/pjsk/sk/line")
	registry.add(runtime != nil && runtime.SK != nil, "sk/query", "/internal/pjsk/sk/query")
	registry.add(runtime != nil && runtime.SK != nil, "sk/check-room", "/internal/pjsk/sk/check-room")
	registry.add(runtime != nil && runtime.SK != nil, "sk/speed", "/internal/pjsk/sk/speed")
	registry.add(runtime != nil && runtime.SK != nil, "sk/player-trace", "/internal/pjsk/sk/player-trace")
	registry.add(runtime != nil && runtime.SK != nil, "sk/rank-trace", "/internal/pjsk/sk/rank-trace")
	registry.add(runtime != nil && runtime.SK != nil, "sk/winrate", "/internal/pjsk/sk/winrate")
	registry.add(runtime != nil && runtime.Score != nil, "score/control", "/internal/pjsk/score/control")
	registry.add(runtime != nil && runtime.Score != nil, "score/custom-room", "/internal/pjsk/score/custom-room")
	registry.add(runtime != nil && runtime.Score != nil, "score/music-meta", "/internal/pjsk/score/music-meta")
	registry.add(runtime != nil && runtime.Score != nil, "score/music-board", "/internal/pjsk/score/music-board")
	registry.add(runtime != nil && runtime.Stamps != nil, "stamp/list", "/internal/pjsk/stamp/list")
	return registry
}

func (r renderDispatchRegistry) add(enabled bool, target string, basePath string) {
	if !enabled {
		return
	}
	r[target] = renderDispatchTarget{
		buildPath:  basePath + "/" + renderDispatchBuild,
		renderPath: basePath + "/" + renderDispatchRender,
	}
}

func (h *RenderDispatchHandler) Dispatch(c fiber.Ctx) error {
	var req RenderDispatchRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	target := normalizeRenderDispatchTarget(req.Target)
	if target == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, "target is required")
	}

	path, err := h.registry.resolve(target, req.Operation)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	payload := bytes.TrimSpace(req.Payload)
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return api.JSONResponse(c, fiber.StatusBadRequest, "payload is required")
	}

	c.Request().Header.SetContentType("application/json")
	c.Request().SetBodyRaw(payload)
	c.Path(path)
	return c.RestartRouting()
}

func (r renderDispatchRegistry) resolve(target string, operation string) (string, error) {
	entry, ok := r[target]
	if !ok {
		return "", fmt.Errorf("unsupported render target: %s", target)
	}

	switch normalizeRenderDispatchOperation(operation) {
	case renderDispatchBuild:
		return entry.buildPath, nil
	case renderDispatchRender:
		return entry.renderPath, nil
	default:
		return "", fmt.Errorf("unsupported render operation: %s", strings.TrimSpace(operation))
	}
}

func normalizeRenderDispatchTarget(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func normalizeRenderDispatchOperation(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
