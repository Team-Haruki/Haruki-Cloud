package pjsk

import (
	"context"
	"errors"

	"haruki-cloud/api"
	onebot11 "haruki-cloud/api/bot/onebot11"
	pjskHandler "haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"

	"github.com/gofiber/fiber/v3"
)

// CommandHandler handles POST /internal/pjsk/command.
//
// Design scope: this endpoint parses internal raw commands and returns whatever
// unified Execute() produces. Today that is still primarily PNG-producing
// render commands; account-binding commands are intentionally absent from the
// GlobalCommandResolver and therefore do not flow through this endpoint.
type CommandHandler struct {
	resolver  *parser.GlobalCommandResolver
	renderApp *renderapp.App
}

// RegisterPJSKCommandRoute registers the bot render-command endpoint under /internal/pjsk.
// Uses the same VerifyAPIAuthorization middleware as other /internal/ routes.
func RegisterPJSKCommandRoute(app *fiber.App, resolver *parser.GlobalCommandResolver, renderApp *renderapp.App) {
	if resolver == nil || renderApp == nil {
		return
	}
	h := &CommandHandler{resolver: resolver, renderApp: renderApp}
	internal := app.Group("/internal/pjsk", api.VerifyAPIAuthorization())
	internal.Post("/command", h.HandleCommand)
}

// HandleCommand parses a raw text render command, routes it through the bridge, and returns PNG.
func (h *CommandHandler) HandleCommand(c fiber.Ctx) error {
	var req CommandRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	if req.Command == "" {
		return api.JSONResponse(c, fiber.StatusBadRequest, "command is required")
	}

	resolved, err := h.resolver.Resolve(req.Command)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, "unrecognized command", CommandErrorResponse{
			Error: err.Error(),
		})
	}
	applyLegacyRequesterContext(resolved, req)

	// Override region if caller specified a server
	if req.Server != "" {
		resolved.Region = req.Server
	}

	responseData, err := pjskHandler.Execute(context.Background(), resolved, h.renderApp)
	if err != nil {
		status, message, data := legacyCommandExecutionError(err, resolved.Mode)
		return api.JSONResponse(c, status, message, data)
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", responseData)
}

func applyLegacyRequesterContext(resolved *parser.ResolvedCommand, req CommandRequest) {
	if resolved == nil {
		return
	}
	resolved.RequesterPlatform = req.IMPlatform
	resolved.RequesterUserID = req.IMUserID
}

func legacyCommandExecutionError(err error, mode string) (int, string, interface{}) {
	var replyErr onebot11.ReplayError
	if errors.As(err, &replyErr) {
		return fiber.StatusOK, "ok", []onebot11.Segment{onebot11.Text(string(replyErr))}
	}
	return fiber.StatusInternalServerError, "render failed", CommandErrorResponse{
		Error: err.Error(),
		Mode:  mode,
	}
}
