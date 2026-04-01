package pjsk

import (
"net/http"

"haruki-cloud/api"
rendereducation "haruki-cloud/internal/pjsk/render/education"
rendermusic "haruki-cloud/internal/pjsk/render/music"
"haruki-cloud/utils/drawing"

"github.com/gofiber/fiber/v3"
)

func (h *RenderHandler) BuildMusicDetail(c fiber.Ctx) error {
	var query rendermusic.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicDetailRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicDetailDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicDetail(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, musicDetailDrawingEndpoint, h.app.Music.BuildMusicDetailRequest, func(client *drawing.HarukiDrawingClient, _ rendermusic.Query, payload *drawing.MusicDetailRequest) ([]byte, error) {
		return client.GenerateMusicDetail(payload)
	})
}

func (h *RenderHandler) BuildMusicBriefList(c fiber.Ctx) error {
	var query rendermusic.BriefListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicBriefListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicBriefDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicBriefList(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, musicBriefDrawingEndpoint, h.app.Music.BuildMusicBriefListRequest, func(client *drawing.HarukiDrawingClient, _ rendermusic.BriefListQuery, payload *drawing.MusicBriefListRequest) ([]byte, error) {
		return client.GenerateMusicBriefList(payload)
	})
}

func (h *RenderHandler) BuildMusicList(c fiber.Ctx) error {
	var query rendermusic.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicListDrawingEndpoint(query.ShowID, query.IncludeLeaks),
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicList(c fiber.Ctx) error {
	return renderBuiltPNGWithEndpoint(h, c, func(query rendermusic.ListQuery, _ *drawing.MusicListRequest) string {
		return musicListDrawingEndpoint(query.ShowID, query.IncludeLeaks)
	}, h.app.Music.BuildMusicListRequest, func(client *drawing.HarukiDrawingClient, query rendermusic.ListQuery, payload *drawing.MusicListRequest) ([]byte, error) {
		return client.GenerateMusicList(payload, query.ShowID, query.IncludeLeaks)
	})
}

func (h *RenderHandler) BuildMusicProgress(c fiber.Ctx) error {
	var query rendermusic.ProgressQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicProgressRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicProgressEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicProgress(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, musicProgressEndpoint, h.app.Music.BuildMusicProgressRequest, func(client *drawing.HarukiDrawingClient, _ rendermusic.ProgressQuery, payload *drawing.PlayProgressRequest) ([]byte, error) {
		return client.GeneratePlayProgress(payload)
	})
}

func (h *RenderHandler) BuildMusicRewardsDetail(c fiber.Ctx) error {
	var query rendermusic.RewardsDetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicRewardsDetailRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicRewardsDetailEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicRewardsDetail(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, musicRewardsDetailEndpoint, h.app.Music.BuildMusicRewardsDetailRequest, func(client *drawing.HarukiDrawingClient, _ rendermusic.RewardsDetailQuery, payload *drawing.DetailMusicRewardsRequest) ([]byte, error) {
		return client.GenerateDetailMusicRewards(payload)
	})
}

func (h *RenderHandler) BuildMusicRewardsBasic(c fiber.Ctx) error {
	var query rendermusic.RewardsBasicQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicRewardsBasicRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicRewardsBasicEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicRewardsBasic(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, musicRewardsBasicEndpoint, h.app.Music.BuildMusicRewardsBasicRequest, func(client *drawing.HarukiDrawingClient, _ rendermusic.RewardsBasicQuery, payload *drawing.BasicMusicRewardsRequest) ([]byte, error) {
		return client.GenerateBasicMusicRewards(payload)
	})
}

func (h *RenderHandler) BuildMusicChart(c fiber.Ctx) error {
	var query rendermusic.ChartQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Music.BuildMusicChartRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: musicChartDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderMusicChart(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, musicChartDrawingEndpoint, h.app.Music.BuildMusicChartRequest, func(client *drawing.HarukiDrawingClient, _ rendermusic.ChartQuery, payload *drawing.GenerateMusicChartRequest) ([]byte, error) {
		return client.GenerateMusicChart(payload)
	})
}

func (h *RenderHandler) BuildEducationPowerBonus(c fiber.Ctx) error {
	var req drawing.PowerBonusDetailRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Edu.BuildPowerBonusDetailRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: educationPowerEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) BuildEducationChallengeLive(c fiber.Ctx) error {
	var query rendereducation.ChallengeLiveQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Edu.BuildChallengeLiveDetailsRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: educationChallengeEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEducationChallengeLive(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, educationChallengeEndpoint, h.app.Edu.BuildChallengeLiveDetailsRequest, func(client *drawing.HarukiDrawingClient, _ rendereducation.ChallengeLiveQuery, payload *drawing.ChallengeLiveDetailsRequest) ([]byte, error) {
		return client.GenerateChallengeLiveDetails(payload)
	})
}

func (h *RenderHandler) RenderEducationPowerBonus(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, educationPowerEndpoint, h.app.Edu.BuildPowerBonusDetailRequest, func(client *drawing.HarukiDrawingClient, _ drawing.PowerBonusDetailRequest, payload *drawing.PowerBonusDetailRequest) ([]byte, error) {
		return client.GeneratePowerBonusDetail(payload)
	})
}

func (h *RenderHandler) BuildEducationAreaItem(c fiber.Ctx) error {
	var req drawing.AreaItemUpgradeMaterialsRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Edu.BuildAreaItemUpgradeMaterialsRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: educationAreaItemEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEducationAreaItem(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, educationAreaItemEndpoint, h.app.Edu.BuildAreaItemUpgradeMaterialsRequest, func(client *drawing.HarukiDrawingClient, _ drawing.AreaItemUpgradeMaterialsRequest, payload *drawing.AreaItemUpgradeMaterialsRequest) ([]byte, error) {
		return client.GenerateAreaItemUpgradeMaterials(payload)
	})
}

func (h *RenderHandler) BuildEducationBonds(c fiber.Ctx) error {
	var req drawing.BondsRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Edu.BuildBondsRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: educationBondsEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEducationBonds(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, educationBondsEndpoint, h.app.Edu.BuildBondsRequest, func(client *drawing.HarukiDrawingClient, _ drawing.BondsRequest, payload *drawing.BondsRequest) ([]byte, error) {
		return client.GenerateBonds(payload)
	})
}

func (h *RenderHandler) BuildEducationLeaderCount(c fiber.Ctx) error {
	var req drawing.LeaderCountRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Edu.BuildLeaderCountRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: educationLeaderEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEducationLeaderCount(c fiber.Ctx) error {
	return renderBuiltPNG(h, c, educationLeaderEndpoint, h.app.Edu.BuildLeaderCountRequest, func(client *drawing.HarukiDrawingClient, _ drawing.LeaderCountRequest, payload *drawing.LeaderCountRequest) ([]byte, error) {
		return client.GenerateLeaderCount(payload)
	})
}
