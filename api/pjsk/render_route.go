package pjsk

import (
	"net/http"

	"haruki-cloud/api"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderevent "haruki-cloud/internal/pjsk/render/event"

	"github.com/gofiber/fiber/v3"
)

const (
	eventDetailDrawingEndpoint = "/api/pjsk/event/detail"
	eventListDrawingEndpoint   = "/api/pjsk/event/list"
)

type RenderHandler struct {
	app *renderapp.App
}

func RegisterPJSKRenderRoutes(app *fiber.App, runtime *renderapp.App) {
	if runtime == nil {
		return
	}

	internal := app.Group("/internal/pjsk", api.VerifyAPIAuthorization())
	registerEventRenderRoutes(internal, runtime)
}

func registerEventRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Events == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/event")
	group.Post("/detail/build", handler.BuildEventDetail)
	group.Post("/detail/render", handler.RenderEventDetail)
	group.Post("/list/build", handler.BuildEventList)
	group.Post("/list/render", handler.RenderEventList)
}

func (h *RenderHandler) BuildEventDetail(c fiber.Ctx) error {
	var query renderevent.DetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Events.BuildEventDetailRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: eventDetailDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEventDetail(c fiber.Ctx) error {
	var query renderevent.DetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Events.RenderEventDetail(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildEventList(c fiber.Ctx) error {
	var query renderevent.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Events.BuildEventListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: eventListDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEventList(c fiber.Ctx) error {
	var query renderevent.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Events.RenderEventList(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}
