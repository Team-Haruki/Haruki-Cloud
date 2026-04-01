package pjsk

import (
"net/http"

"haruki-cloud/api"
rendersk "haruki-cloud/internal/pjsk/render/sk"
renderstamp "haruki-cloud/internal/pjsk/render/stamp"
"haruki-cloud/utils/drawing"

"github.com/gofiber/fiber/v3"
)

func (h *RenderHandler) BuildSKLine(c fiber.Ctx) error {
	var req rendersk.LineRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildLineRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skLineEndpoint(req.Full),
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKLine(c fiber.Ctx) error {
	return renderBuiltPNGWithEndpoint(h, c, func(req rendersk.LineRequest, _ *rendersk.LineRequest) string {
		return skLineEndpoint(req.Full)
	}, h.app.SK.BuildLineRequest, func(client *drawing.HarukiDrawingClient, _ rendersk.LineRequest, payload *rendersk.LineRequest) ([]byte, error) {
		return client.GenerateSKLine(&payload.SklRequest, payload.Full)
	})
}

func (h *RenderHandler) BuildSKLineFromTracker(c fiber.Ctx) error {
	var req rendersk.TrackerRankQuery
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildLineRequestFromTracker(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skLineEndpoint(payload.Full),
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKLineFromTracker(c fiber.Ctx) error {
	return renderBuiltPNGWithEndpoint(h, c, func(req rendersk.TrackerRankQuery, payload *rendersk.LineRequest) string {
		if payload != nil {
			return skLineEndpoint(payload.Full)
		}
		return skLineEndpoint(req.Full)
	}, h.app.SK.BuildLineRequestFromTracker, func(client *drawing.HarukiDrawingClient, _ rendersk.TrackerRankQuery, payload *rendersk.LineRequest) ([]byte, error) {
		return client.GenerateSKLine(&payload.SklRequest, payload.Full)
	})
}

func (h *RenderHandler) BuildSKQuery(c fiber.Ctx) error {
	var req drawing.SKRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildQueryRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skQueryEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKQuery(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skQueryEndpoint, h.app.SK.BuildQueryRequest, func(client *drawing.HarukiDrawingClient, _ drawing.SKRequest, payload *drawing.SKRequest) ([]byte, error) {
		return client.GenerateSKQuery(payload)
	})
}

func (h *RenderHandler) BuildSKQueryFromTracker(c fiber.Ctx) error {
	var req rendersk.TrackerRankQuery
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildQueryRequestFromTracker(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skQueryEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKQueryFromTracker(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skQueryEndpoint, h.app.SK.BuildQueryRequestFromTracker, func(client *drawing.HarukiDrawingClient, _ rendersk.TrackerRankQuery, payload *drawing.SKRequest) ([]byte, error) {
		return client.GenerateSKQuery(payload)
	})
}

func (h *RenderHandler) BuildSKCheckRoom(c fiber.Ctx) error {
	var req drawing.CFRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildCheckRoomRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skCheckRoomEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKCheckRoom(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skCheckRoomEndpoint, h.app.SK.BuildCheckRoomRequest, func(client *drawing.HarukiDrawingClient, _ drawing.CFRequest, payload *drawing.CFRequest) ([]byte, error) {
		return client.GenerateSKCheckRoom(payload)
	})
}

func (h *RenderHandler) BuildSKCheckRoomFromTracker(c fiber.Ctx) error {
	var req rendersk.TrackerRankQuery
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildCheckRoomRequestFromTracker(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skCheckRoomEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKCheckRoomFromTracker(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skCheckRoomEndpoint, h.app.SK.BuildCheckRoomRequestFromTracker, func(client *drawing.HarukiDrawingClient, _ rendersk.TrackerRankQuery, payload *drawing.CFRequest) ([]byte, error) {
		return client.GenerateSKCheckRoom(payload)
	})
}

func (h *RenderHandler) BuildSKSpeed(c fiber.Ctx) error {
	var req drawing.SpeedRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildSpeedRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skSpeedEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKSpeed(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skSpeedEndpoint, h.app.SK.BuildSpeedRequest, func(client *drawing.HarukiDrawingClient, _ drawing.SpeedRequest, payload *drawing.SpeedRequest) ([]byte, error) {
		return client.GenerateSKSpeed(payload)
	})
}

func (h *RenderHandler) BuildSKSpeedFromTracker(c fiber.Ctx) error {
	var req rendersk.TrackerRankQuery
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildSpeedRequestFromTracker(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skSpeedEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKSpeedFromTracker(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skSpeedEndpoint, h.app.SK.BuildSpeedRequestFromTracker, func(client *drawing.HarukiDrawingClient, _ rendersk.TrackerRankQuery, payload *drawing.SpeedRequest) ([]byte, error) {
		return client.GenerateSKSpeed(payload)
	})
}

func (h *RenderHandler) BuildSKPlayerTrace(c fiber.Ctx) error {
	var req drawing.PlayerTraceRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildPlayerTraceRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skPlayerTraceEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKPlayerTrace(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skPlayerTraceEndpoint, h.app.SK.BuildPlayerTraceRequest, func(client *drawing.HarukiDrawingClient, _ drawing.PlayerTraceRequest, payload *drawing.PlayerTraceRequest) ([]byte, error) {
		return client.GenerateSKPlayerTrace(payload)
	})
}

func (h *RenderHandler) BuildSKRankTrace(c fiber.Ctx) error {
	var req drawing.RankTraceRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildRankTraceRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skRankTraceEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKRankTrace(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skRankTraceEndpoint, h.app.SK.BuildRankTraceRequest, func(client *drawing.HarukiDrawingClient, _ drawing.RankTraceRequest, payload *drawing.RankTraceRequest) ([]byte, error) {
		return client.GenerateSKRankTrace(payload)
	})
}

func (h *RenderHandler) BuildSKRankTraceFromTracker(c fiber.Ctx) error {
	var req rendersk.TrackerRankQuery
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildRankTraceRequestFromTracker(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skRankTraceEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKRankTraceFromTracker(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skRankTraceEndpoint, h.app.SK.BuildRankTraceRequestFromTracker, func(client *drawing.HarukiDrawingClient, _ rendersk.TrackerRankQuery, payload *drawing.RankTraceRequest) ([]byte, error) {
		return client.GenerateSKRankTrace(payload)
	})
}

func (h *RenderHandler) BuildSKWinRate(c fiber.Ctx) error {
	var req drawing.WinRateRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.SK.BuildWinRateRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: skWinRateEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderSKWinRate(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, skWinRateEndpoint, h.app.SK.BuildWinRateRequest, func(client *drawing.HarukiDrawingClient, _ drawing.WinRateRequest, payload *drawing.WinRateRequest) ([]byte, error) {
		return client.GenerateSKWinRate(payload)
	})
}

func (h *RenderHandler) BuildScoreControl(c fiber.Ctx) error {
	var req drawing.ScoreControlRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Score.BuildScoreControlRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: scoreControlEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderScoreControl(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, scoreControlEndpoint, h.app.Score.BuildScoreControlRequest, func(client *drawing.HarukiDrawingClient, _ drawing.ScoreControlRequest, payload *drawing.ScoreControlRequest) ([]byte, error) {
		return client.GenerateScoreControl(payload)
	})
}

func (h *RenderHandler) BuildCustomRoomScore(c fiber.Ctx) error {
	var req drawing.CustomRoomScoreRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Score.BuildCustomRoomScoreRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: scoreCustomRoomEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderCustomRoomScore(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, scoreCustomRoomEndpoint, h.app.Score.BuildCustomRoomScoreRequest, func(client *drawing.HarukiDrawingClient, _ drawing.CustomRoomScoreRequest, payload *drawing.CustomRoomScoreRequest) ([]byte, error) {
		return client.GenerateCustomRoomScore(payload)
	})
}

func (h *RenderHandler) BuildMusicMeta(c fiber.Ctx) error {
	var req []drawing.MusicMetaRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Score.BuildMusicMetaRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: scoreMusicMetaEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicMeta(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, scoreMusicMetaEndpoint, h.app.Score.BuildMusicMetaRequest, func(client *drawing.HarukiDrawingClient, _ []drawing.MusicMetaRequest, payload []drawing.MusicMetaRequest) ([]byte, error) {
		return client.GenerateMusicMeta(payload)
	})
}

func (h *RenderHandler) BuildMusicBoard(c fiber.Ctx) error {
	var req drawing.MusicBoardRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Score.BuildMusicBoardRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: scoreMusicBoardEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicBoard(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, scoreMusicBoardEndpoint, h.app.Score.BuildMusicBoardRequest, func(client *drawing.HarukiDrawingClient, _ drawing.MusicBoardRequest, payload *drawing.MusicBoardRequest) ([]byte, error) {
		return client.GenerateMusicBoard(payload)
	})
}

func (h *RenderHandler) BuildStampList(c fiber.Ctx) error {
	var query renderstamp.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Stamps.BuildStampListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: stampListDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderStampList(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, stampListDrawingEndpoint, h.app.Stamps.BuildStampListRequest, func(client *drawing.HarukiDrawingClient, _ renderstamp.ListQuery, payload *drawing.StampListRequest) ([]byte, error) {
		return client.GenerateStampList(payload)
	})
}
