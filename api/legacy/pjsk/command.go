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
// Design scope: this endpoint handles RENDER commands only — commands that
// produce a PNG image (card queries, event info, SK lines, etc.).
//
// Account management commands (bind, unbind, set-main, etc.) are NOT handled
// here. Those are data write operations with no image output and should be
// exposed as dedicated REST endpoints when their business logic is implemented.
// They are intentionally absent from GlobalCommandResolver.
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

	pngBytes, err := pjskHandler.Execute(context.Background(), resolved, h.renderApp)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusInternalServerError, "render failed", CommandErrorResponse{
			Error: err.Error(),
			Mode:  resolved.Mode,
		})
	}

	c.Set("Content-Type", "image/png")
	return c.Send(pngBytes)
}
