package groupguard

import (
	"errors"
	"strings"

	"haruki-cloud/api"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"

	"github.com/gofiber/fiber/v3"
)

type bindingCheckRequest struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
}

type bindingCheckBatchRequest struct {
	Platform        string   `json:"platform"`
	PlatformUserIDs []string `json:"platform_user_ids"`
}

type BindingInfo struct {
	Server     string `json:"server"`
	GameUserID string `json:"game_user_id"`
}

type BindingCheckResult struct {
	Bound    bool          `json:"bound"`
	Bindings []BindingInfo `json:"bindings"`
	Banned   bool          `json:"banned"`
}

type BindingCheckBatchResponse struct {
	Results map[string]BindingCheckResult `json:"results"`
}

type Handler struct {
	toolbox *sekaiapi.HarukiToolboxClient
}

func RegisterGroupGuardRoutes(app *fiber.App, toolbox *sekaiapi.HarukiToolboxClient) {
	group := app.Group("/api/internal/group-guard", api.VerifyAPIAuthorization())
	handler := &Handler{toolbox: toolbox}
	group.Post("/binding/check", handler.CheckBinding)
	group.Post("/binding/check-batch", handler.CheckBindingBatch)
}

func (h *Handler) CheckBinding(c fiber.Ctx) error {
	var req bindingCheckRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}
	req.Platform = strings.TrimSpace(req.Platform)
	req.PlatformUserID = strings.TrimSpace(req.PlatformUserID)
	if req.Platform == "" || req.PlatformUserID == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrMissingPlatformInfo)
	}

	result, err := h.lookupBinding(req.Platform, req.PlatformUserID)
	if err != nil {
		return respondBindingLookupError(c, err)
	}
	return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, result)
}

func (h *Handler) CheckBindingBatch(c fiber.Ctx) error {
	var req bindingCheckBatchRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}
	req.Platform = strings.TrimSpace(req.Platform)
	if req.Platform == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrMissingPlatformInfo)
	}

	results := make(map[string]BindingCheckResult)
	seen := make(map[string]struct{})
	for _, rawID := range req.PlatformUserIDs {
		platformUserID := strings.TrimSpace(rawID)
		if platformUserID == "" {
			continue
		}
		if _, ok := seen[platformUserID]; ok {
			continue
		}
		seen[platformUserID] = struct{}{}

		result, err := h.lookupBinding(req.Platform, platformUserID)
		if err != nil {
			return respondBindingLookupError(c, err)
		}
		results[platformUserID] = result
	}
	if len(results) == 0 {
		return api.JSONResponse(c, fiber.StatusBadRequest, "platform_user_ids is required")
	}
	return api.JSONResponse(c, fiber.StatusOK, api.ResponseOK, BindingCheckBatchResponse{
		Results: results,
	})
}

func (h *Handler) lookupBinding(platform string, platformUserID string) (BindingCheckResult, error) {
	result := BindingCheckResult{
		Bound:    false,
		Bindings: []BindingInfo{},
		Banned:   false,
	}
	if h == nil || h.toolbox == nil {
		return result, sekaiapi.ErrClientNotConfigured
	}

	bindings, err := h.toolbox.GetToolboxUserFastVerificationGameAccountBindings(platform, platformUserID)
	if err == nil {
		result.Bindings = convertBindings(bindings)
		result.Bound = len(result.Bindings) > 0
		return result, nil
	}

	switch {
	case errors.Is(err, sekaiapi.ErrInvalidPlatformUser),
		errors.Is(err, sekaiapi.ErrAccountBindingNotFound):
		return result, nil
	case errors.Is(err, sekaiapi.ErrAccountOwnerBanned):
		result.Banned = true
		return result, nil
	default:
		return result, err
	}
}

func convertBindings(bindings []sekaiapi.UserGameBinding) []BindingInfo {
	if len(bindings) == 0 {
		return []BindingInfo{}
	}
	out := make([]BindingInfo, 0, len(bindings))
	for _, item := range bindings {
		server := strings.TrimSpace(item.Server)
		gameUserID := strings.TrimSpace(item.GameUserID)
		if server == "" || gameUserID == "" {
			continue
		}
		out = append(out, BindingInfo{
			Server:     server,
			GameUserID: gameUserID,
		})
	}
	return out
}

func respondBindingLookupError(c fiber.Ctx, err error) error {
	if err == nil {
		return api.InternalError(c)
	}
	status := fiber.StatusBadGateway
	message := "查询绑定状态失败，请稍后再试"
	switch {
	case errors.Is(err, sekaiapi.ErrClientNotConfigured):
		status = fiber.StatusServiceUnavailable
		message = "绑定查询服务未就绪，请稍后再试"
	default:
		var toolboxErr *sekaiapi.ToolboxAPIError
		if errors.As(err, &toolboxErr) && toolboxErr.StatusCode == fiber.StatusServiceUnavailable {
			status = fiber.StatusServiceUnavailable
		}
	}
	return api.JSONResponse(c, status, message)
}
