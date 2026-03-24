package pjsk

import (
	"context"

	"haruki-cloud/api"
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

	// Override region if caller specified a server
	if req.Server != "" {
		resolved.Region = req.Server
	}

	responseData, dataType, err := pjskHandler.Execute(context.Background(), resolved, h.renderApp)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusInternalServerError, "render failed", CommandErrorResponse{
			Error: err.Error(),
			Mode:  resolved.Mode,
		})
	}

	switch dataType {
	case pjskHandler.CommandResultDataTypeImagePNG:
		c.Set("Content-Type", string(dataType))
		return c.Send(responseData)
	case pjskHandler.CommandResultDataTypeText:
		return api.JSONResponse(c, fiber.StatusOK, string(responseData))
	default:
		return api.JSONResponse(c, fiber.StatusInternalServerError, "unsupported command result", CommandErrorResponse{
			Error: "unsupported execute result data type",
			Mode:  resolved.Mode,
		})
	}
}
