package pjsk

import (
"fmt"
"net/http"

"haruki-cloud/api"
rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
"haruki-cloud/utils/drawing"

"github.com/gofiber/fiber/v3"
)

func (h *RenderHandler) BuildMysekaiResource(c fiber.Ctx) error {
	var query rendermysekai.ResourceQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildResourceRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiResourceEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiResource(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiResourceEndpoint, h.app.MySekai.BuildResourceRequest, func(client *drawing.HarukiDrawingClient, _ rendermysekai.ResourceQuery, payload *drawing.MysekaiResourceRequest) ([]byte, error) {
		return client.GenerateMysekaiResource(payload)
	})
}

func (h *RenderHandler) BuildMysekaiMap(c fiber.Ctx) error {
	var query rendermysekai.MapQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildMapRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiMapEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiMap(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiMapEndpoint, h.app.MySekai.BuildMapRequest, func(client *drawing.HarukiDrawingClient, _ rendermysekai.MapQuery, payload *drawing.MysekaiMsrMapRequest) ([]byte, error) {
		return client.GenerateMysekaiMap(payload)
	})
}

func (h *RenderHandler) BuildMysekaiFixtureList(c fiber.Ctx) error {
	var query rendermysekai.FixtureListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildFixtureListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiFixtureListEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiFixtureList(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiFixtureListEndpoint, h.app.MySekai.BuildFixtureListRequest, func(client *drawing.HarukiDrawingClient, _ rendermysekai.FixtureListQuery, payload *drawing.MysekaiFixtureListRequest) ([]byte, error) {
		return client.GenerateMysekaiFixtureList(payload)
	})
}

func (h *RenderHandler) BuildMysekaiFixtureDetail(c fiber.Ctx) error {
	var query rendermysekai.FixtureDetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildFixtureDetailRequests(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiFixtureDetailEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiFixtureDetail(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiFixtureDetailEndpoint, func(query rendermysekai.FixtureDetailQuery) (*drawing.MysekaiFixtureDetailRequest, error) {
		requests, err := h.app.MySekai.BuildFixtureDetailRequests(query)
		if err != nil {
			return nil, err
		}
		if len(requests) != 1 {
			return nil, fmt.Errorf("mysekai fixture detail render requires exactly one fixture id")
		}
		return &requests[0], nil
	}, func(client *drawing.HarukiDrawingClient, _ rendermysekai.FixtureDetailQuery, payload *drawing.MysekaiFixtureDetailRequest) ([]byte, error) {
		return client.GenerateMysekaiFixtureDetail(payload)
	})
}

func (h *RenderHandler) BuildMysekaiDoorUpgrade(c fiber.Ctx) error {
	var query rendermysekai.DoorUpgradeQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildDoorUpgradeRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiDoorUpgradeEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiDoorUpgrade(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiDoorUpgradeEndpoint, h.app.MySekai.BuildDoorUpgradeRequest, func(client *drawing.HarukiDrawingClient, _ rendermysekai.DoorUpgradeQuery, payload *drawing.MysekaiDoorUpgradeRequest) ([]byte, error) {
		return client.GenerateMysekaiDoorUpgrade(payload)
	})
}

func (h *RenderHandler) BuildMysekaiMusicRecord(c fiber.Ctx) error {
	var query rendermysekai.MusicRecordQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildMusicRecordRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiMusicRecordEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiMusicRecord(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiMusicRecordEndpoint, h.app.MySekai.BuildMusicRecordRequest, func(client *drawing.HarukiDrawingClient, _ rendermysekai.MusicRecordQuery, payload *drawing.MysekaiMusicrecordRequest) ([]byte, error) {
		return client.GenerateMysekaiMusicRecord(payload)
	})
}

func (h *RenderHandler) BuildMysekaiTalkList(c fiber.Ctx) error {
	var query rendermysekai.TalkListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.MySekai.BuildTalkListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: mysekaiTalkListEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMysekaiTalkList(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, mysekaiTalkListEndpoint, h.app.MySekai.BuildTalkListRequest, func(client *drawing.HarukiDrawingClient, _ rendermysekai.TalkListQuery, payload *drawing.MysekaiTalkListRequest) ([]byte, error) {
		return client.GenerateMysekaiTalkList(payload)
	})
}
