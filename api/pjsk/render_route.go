package pjsk

import (
	"fmt"
	"net/http"

	"haruki-cloud/api"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendergacha "haruki-cloud/internal/pjsk/render/gacha"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/utils/drawing"

	"github.com/gofiber/fiber/v3"
)

const (
	cardDetailDrawingEndpoint  = "/api/pjsk/card/detail"
	cardListDrawingEndpoint    = "/api/pjsk/card/list"
	cardBoxDrawingEndpoint     = "/api/pjsk/card/box"
	eventDetailDrawingEndpoint = "/api/pjsk/event/detail"
	eventListDrawingEndpoint   = "/api/pjsk/event/list"
	gachaDetailDrawingEndpoint = "/api/pjsk/gacha/detail"
	gachaListDrawingEndpoint   = "/api/pjsk/gacha/list"
	honorDrawingEndpoint       = "/api/pjsk/honor"
	charaBirthdayEndpoint      = "/api/pjsk/misc/chara-birthday"
	musicDetailDrawingEndpoint = "/api/pjsk/music/detail"
	musicBriefDrawingEndpoint  = "/api/pjsk/music/brief-list"
	musicChartDrawingEndpoint  = "/api/pjsk/chart"
	scoreControlEndpoint       = "/api/pjsk/score/control"
	scoreCustomRoomEndpoint    = "/api/pjsk/score/custom-room"
	scoreMusicMetaEndpoint     = "/api/pjsk/score/music-meta"
	scoreMusicBoardEndpoint    = "/api/pjsk/score/music-board"
	stampListDrawingEndpoint   = "/api/pjsk/stamp/list"
)

func musicListDrawingEndpoint(showID bool, showLeak bool) string {
	return fmt.Sprintf("/api/pjsk/music/list?show_id=%t&show_leak=%t", showID, showLeak)
}

type RenderHandler struct {
	app *renderapp.App
}

func RegisterPJSKRenderRoutes(app *fiber.App, runtime *renderapp.App) {
	if runtime == nil {
		return
	}

	internal := app.Group("/internal/pjsk", api.VerifyAPIAuthorization())
	registerCardRenderRoutes(internal, runtime)
	registerEventRenderRoutes(internal, runtime)
	registerGachaRenderRoutes(internal, runtime)
	registerHonorRenderRoutes(internal, runtime)
	registerMiscRenderRoutes(internal, runtime)
	registerMusicRenderRoutes(internal, runtime)
	registerScoreRenderRoutes(internal, runtime)
	registerStampRenderRoutes(internal, runtime)
}

func registerCardRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Cards == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/card")
	group.Post("/detail/build", handler.BuildCardDetail)
	group.Post("/detail/render", handler.RenderCardDetail)
	group.Post("/list/build", handler.BuildCardList)
	group.Post("/list/render", handler.RenderCardList)
	group.Post("/box/build", handler.BuildCardBox)
	group.Post("/box/render", handler.RenderCardBox)
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

func registerGachaRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Gachas == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/gacha")
	group.Post("/detail/build", handler.BuildGachaDetail)
	group.Post("/detail/render", handler.RenderGachaDetail)
	group.Post("/list/build", handler.BuildGachaList)
	group.Post("/list/render", handler.RenderGachaList)
}

func registerStampRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Stamps == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/stamp")
	group.Post("/list/build", handler.BuildStampList)
	group.Post("/list/render", handler.RenderStampList)
}

func registerHonorRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Honors == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/honor")
	group.Post("/build", handler.BuildHonor)
	group.Post("/render", handler.RenderHonor)
}

func registerMiscRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Misc == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/misc")
	group.Post("/chara-birthday/build", handler.BuildCharaBirthday)
	group.Post("/chara-birthday/render", handler.RenderCharaBirthday)
}

func registerMusicRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Music == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/music")
	group.Post("/detail/build", handler.BuildMusicDetail)
	group.Post("/detail/render", handler.RenderMusicDetail)
	group.Post("/brief-list/build", handler.BuildMusicBriefList)
	group.Post("/brief-list/render", handler.RenderMusicBriefList)
	group.Post("/list/build", handler.BuildMusicList)
	group.Post("/list/render", handler.RenderMusicList)
	group.Post("/chart/build", handler.BuildMusicChart)
	group.Post("/chart/render", handler.RenderMusicChart)
}

func registerScoreRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Score == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/score")
	group.Post("/control/build", handler.BuildScoreControl)
	group.Post("/control/render", handler.RenderScoreControl)
	group.Post("/custom-room/build", handler.BuildCustomRoomScore)
	group.Post("/custom-room/render", handler.RenderCustomRoomScore)
	group.Post("/music-meta/build", handler.BuildMusicMeta)
	group.Post("/music-meta/render", handler.RenderMusicMeta)
	group.Post("/music-board/build", handler.BuildMusicBoard)
	group.Post("/music-board/render", handler.RenderMusicBoard)
}

func (h *RenderHandler) BuildCardDetail(c fiber.Ctx) error {
	var query rendercard.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Cards.BuildCardDetailRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: cardDetailDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderCardDetail(c fiber.Ctx) error {
	var query rendercard.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Cards.RenderCardDetail(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildCardList(c fiber.Ctx) error {
	var req rendercard.ListRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Cards.BuildCardListRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: cardListDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderCardList(c fiber.Ctx) error {
	var req rendercard.ListRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Cards.RenderCardList(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildCardBox(c fiber.Ctx) error {
	var queries []rendercard.Query
	if err := c.Bind().Body(&queries); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Cards.BuildCardBoxRequest(queries)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: cardBoxDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderCardBox(c fiber.Ctx) error {
	var queries []rendercard.Query
	if err := c.Bind().Body(&queries); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Cards.RenderCardBox(queries)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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

func (h *RenderHandler) BuildGachaDetail(c fiber.Ctx) error {
	var query rendergacha.DetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Gachas.BuildGachaDetailRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: gachaDetailDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderGachaDetail(c fiber.Ctx) error {
	var query rendergacha.DetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Gachas.RenderGachaDetail(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildGachaList(c fiber.Ctx) error {
	var query rendergacha.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Gachas.BuildGachaListRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: gachaListDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderGachaList(c fiber.Ctx) error {
	var query rendergacha.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Gachas.RenderGachaList(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildHonor(c fiber.Ctx) error {
	var query renderhonor.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Honors.BuildHonorRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: honorDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderHonor(c fiber.Ctx) error {
	var query renderhonor.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Honors.RenderHonor(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildCharaBirthday(c fiber.Ctx) error {
	var req drawing.CharaBirthdayRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Misc.BuildCharaBirthdayRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: charaBirthdayEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderCharaBirthday(c fiber.Ctx) error {
	var req drawing.CharaBirthdayRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Misc.RenderCharaBirthday(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

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
	var query rendermusic.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicDetail(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermusic.BriefListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicBriefList(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermusic.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicList(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermusic.ChartQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicChart(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.ScoreControlRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Score.RenderScoreControl(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.CustomRoomScoreRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Score.RenderCustomRoomScore(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req []drawing.MusicMetaRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Score.RenderMusicMeta(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.MusicBoardRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Score.RenderMusicBoard(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query renderstamp.ListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Stamps.RenderStampList(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}

	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}
