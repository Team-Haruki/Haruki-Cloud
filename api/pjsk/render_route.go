package pjsk

import (
	"fmt"
	"net/http"

	"haruki-cloud/api"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	renderdeck "haruki-cloud/internal/pjsk/render/deck"
	rendereducation "haruki-cloud/internal/pjsk/render/education"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	rendergacha "haruki-cloud/internal/pjsk/render/gacha"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	renderprofile "haruki-cloud/internal/pjsk/render/profile"
	rendersk "haruki-cloud/internal/pjsk/render/sk"
	renderstamp "haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/utils/drawing"

	"github.com/gofiber/fiber/v3"
)

const (
	cardDetailDrawingEndpoint    = "/api/pjsk/card/detail"
	cardListDrawingEndpoint      = "/api/pjsk/card/list"
	cardBoxDrawingEndpoint       = "/api/pjsk/card/box"
	deckRecommendEndpoint        = "/api/pjsk/deck/recommend"
	eventDetailDrawingEndpoint   = "/api/pjsk/event/detail"
	eventListDrawingEndpoint     = "/api/pjsk/event/list"
	eventRecordDrawingEndpoint   = "/api/pjsk/event/record"
	gachaDetailDrawingEndpoint   = "/api/pjsk/gacha/detail"
	gachaListDrawingEndpoint     = "/api/pjsk/gacha/list"
	honorDrawingEndpoint         = "/api/pjsk/honor"
	profileDrawingEndpoint       = "/api/pjsk/profile/profile"
	charaBirthdayEndpoint        = "/api/pjsk/misc/chara-birthday"
	mysekaiResourceEndpoint      = "/api/pjsk/mysekai/resource"
	mysekaiFixtureListEndpoint   = "/api/pjsk/mysekai/fixture-list"
	mysekaiFixtureDetailEndpoint = "/api/pjsk/mysekai/fixture-detail"
	mysekaiDoorUpgradeEndpoint   = "/api/pjsk/mysekai/door-upgrade"
	mysekaiMusicRecordEndpoint   = "/api/pjsk/mysekai/music-record"
	mysekaiTalkListEndpoint      = "/api/pjsk/mysekai/talk-list"
	musicDetailDrawingEndpoint   = "/api/pjsk/music/detail"
	musicBriefDrawingEndpoint    = "/api/pjsk/music/brief-list"
	musicProgressEndpoint        = "/api/pjsk/music/progress"
	musicRewardsDetailEndpoint   = "/api/pjsk/music/rewards/detail"
	musicRewardsBasicEndpoint    = "/api/pjsk/music/rewards/basic"
	musicChartDrawingEndpoint    = "/api/pjsk/chart"
	educationChallengeEndpoint   = "/api/pjsk/education/challenge-live"
	educationPowerEndpoint       = "/api/pjsk/education/power-bonus"
	educationAreaItemEndpoint    = "/api/pjsk/education/area-item"
	educationBondsEndpoint       = "/api/pjsk/education/bonds"
	educationLeaderEndpoint      = "/api/pjsk/education/leader-count"
	skQueryEndpoint              = "/api/pjsk/sk/query"
	skCheckRoomEndpoint          = "/api/pjsk/sk/check-room"
	skSpeedEndpoint              = "/api/pjsk/sk/speed"
	skPlayerTraceEndpoint        = "/api/pjsk/sk/player-trace"
	skRankTraceEndpoint          = "/api/pjsk/sk/rank-trace"
	skWinRateEndpoint            = "/api/pjsk/sk/winrate"
	scoreControlEndpoint         = "/api/pjsk/score/control"
	scoreCustomRoomEndpoint      = "/api/pjsk/score/custom-room"
	scoreMusicMetaEndpoint       = "/api/pjsk/score/music-meta"
	scoreMusicBoardEndpoint      = "/api/pjsk/score/music-board"
	stampListDrawingEndpoint     = "/api/pjsk/stamp/list"
)

func musicListDrawingEndpoint(showID bool, showLeak bool) string {
	return fmt.Sprintf("/api/pjsk/music/list?show_id=%t&show_leak=%t", showID, showLeak)
}

func skLineEndpoint(full bool) string {
	return fmt.Sprintf("/api/pjsk/sk/line?full=%t", full)
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
	registerDeckRenderRoutes(internal, runtime)
	registerEventRenderRoutes(internal, runtime)
	registerGachaRenderRoutes(internal, runtime)
	registerHonorRenderRoutes(internal, runtime)
	registerProfileRenderRoutes(internal, runtime)
	registerMiscRenderRoutes(internal, runtime)
	registerMysekaiRenderRoutes(internal, runtime)
	registerMusicRenderRoutes(internal, runtime)
	registerEducationRenderRoutes(internal, runtime)
	registerSKRenderRoutes(internal, runtime)
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

func registerDeckRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Decks == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/deck")
	group.Post("/recommend/build", handler.BuildDeckRecommend)
	group.Post("/recommend/render", handler.RenderDeckRecommend)
	group.Post("/recommend/auto/build", handler.BuildDeckRecommendAuto)
	group.Post("/recommend/auto/render", handler.RenderDeckRecommendAuto)
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
	group.Post("/record/build", handler.BuildEventRecord)
	group.Post("/record/render", handler.RenderEventRecord)
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

func registerProfileRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Profiles == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/profile")
	group.Post("/build", handler.BuildProfile)
	group.Post("/render", handler.RenderProfile)
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

func registerMysekaiRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.MySekai == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/mysekai")
	group.Post("/resource/build", handler.BuildMysekaiResource)
	group.Post("/resource/render", handler.RenderMysekaiResource)
	group.Post("/fixture-list/build", handler.BuildMysekaiFixtureList)
	group.Post("/fixture-list/render", handler.RenderMysekaiFixtureList)
	group.Post("/fixture-detail/build", handler.BuildMysekaiFixtureDetail)
	group.Post("/fixture-detail/render", handler.RenderMysekaiFixtureDetail)
	group.Post("/door-upgrade/build", handler.BuildMysekaiDoorUpgrade)
	group.Post("/door-upgrade/render", handler.RenderMysekaiDoorUpgrade)
	group.Post("/music-record/build", handler.BuildMysekaiMusicRecord)
	group.Post("/music-record/render", handler.RenderMysekaiMusicRecord)
	group.Post("/talk-list/build", handler.BuildMysekaiTalkList)
	group.Post("/talk-list/render", handler.RenderMysekaiTalkList)
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
	group.Post("/progress/build", handler.BuildMusicProgress)
	group.Post("/progress/render", handler.RenderMusicProgress)
	group.Post("/rewards/detail/build", handler.BuildMusicRewardsDetail)
	group.Post("/rewards/detail/render", handler.RenderMusicRewardsDetail)
	group.Post("/rewards/basic/build", handler.BuildMusicRewardsBasic)
	group.Post("/rewards/basic/render", handler.RenderMusicRewardsBasic)
	group.Post("/chart/build", handler.BuildMusicChart)
	group.Post("/chart/render", handler.RenderMusicChart)
}

func registerEducationRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.Edu == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/education")
	group.Post("/challenge-live/build", handler.BuildEducationChallengeLive)
	group.Post("/challenge-live/render", handler.RenderEducationChallengeLive)
	group.Post("/power-bonus/build", handler.BuildEducationPowerBonus)
	group.Post("/power-bonus/render", handler.RenderEducationPowerBonus)
	group.Post("/area-item/build", handler.BuildEducationAreaItem)
	group.Post("/area-item/render", handler.RenderEducationAreaItem)
	group.Post("/bonds/build", handler.BuildEducationBonds)
	group.Post("/bonds/render", handler.RenderEducationBonds)
	group.Post("/leader-count/build", handler.BuildEducationLeaderCount)
	group.Post("/leader-count/render", handler.RenderEducationLeaderCount)
}

func registerSKRenderRoutes(router fiber.Router, runtime *renderapp.App) {
	if runtime == nil || runtime.SK == nil {
		return
	}

	handler := &RenderHandler{app: runtime}
	group := router.Group("/sk")
	group.Post("/line/build", handler.BuildSKLine)
	group.Post("/line/render", handler.RenderSKLine)
	group.Post("/query/build", handler.BuildSKQuery)
	group.Post("/query/render", handler.RenderSKQuery)
	group.Post("/check-room/build", handler.BuildSKCheckRoom)
	group.Post("/check-room/render", handler.RenderSKCheckRoom)
	group.Post("/speed/build", handler.BuildSKSpeed)
	group.Post("/speed/render", handler.RenderSKSpeed)
	group.Post("/player-trace/build", handler.BuildSKPlayerTrace)
	group.Post("/player-trace/render", handler.RenderSKPlayerTrace)
	group.Post("/rank-trace/build", handler.BuildSKRankTrace)
	group.Post("/rank-trace/render", handler.RenderSKRankTrace)
	group.Post("/winrate/build", handler.BuildSKWinRate)
	group.Post("/winrate/render", handler.RenderSKWinRate)
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

func (h *RenderHandler) BuildDeckRecommend(c fiber.Ctx) error {
	var req drawing.DeckRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Decks.BuildRecommendRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: deckRecommendEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderDeckRecommend(c fiber.Ctx) error {
	var req drawing.DeckRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Decks.RenderRecommend(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) BuildDeckRecommendAuto(c fiber.Ctx) error {
	var query renderdeck.AutoQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Decks.BuildAutoRecommendRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: deckRecommendEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderDeckRecommendAuto(c fiber.Ctx) error {
	var query renderdeck.AutoQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Decks.RenderAutoRecommend(query)
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

func (h *RenderHandler) BuildEventRecord(c fiber.Ctx) error {
	var req drawing.EventRecordRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Events.BuildEventRecordRequest(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: eventRecordDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderEventRecord(c fiber.Ctx) error {
	var req drawing.EventRecordRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Events.RenderEventRecord(req)
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

func (h *RenderHandler) BuildProfile(c fiber.Ctx) error {
	var query renderprofile.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	payload, err := h.app.Profiles.BuildProfileRequest(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	return api.JSONResponse(c, fiber.StatusOK, "ok", BuildResponse{
		Endpoint: profileDrawingEndpoint,
		Method:   http.MethodPost,
		Payload:  payload,
	})
}

func (h *RenderHandler) RenderProfile(c fiber.Ctx) error {
	var query renderprofile.Query
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Profiles.RenderProfile(query)
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
	var query rendermysekai.ResourceQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.MySekai.RenderResource(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermysekai.FixtureListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.MySekai.RenderFixtureList(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermysekai.FixtureDetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.MySekai.RenderFixtureDetail(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermysekai.DoorUpgradeQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.MySekai.RenderDoorUpgrade(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermysekai.MusicRecordQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.MySekai.RenderMusicRecord(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermysekai.TalkListQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.MySekai.RenderTalkList(query)
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
	var query rendermusic.ProgressQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicProgress(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermusic.RewardsDetailQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicRewardsDetail(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var query rendermusic.RewardsBasicQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Music.RenderMusicRewardsBasic(query)
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
	var query rendereducation.ChallengeLiveQuery
	if err := c.Bind().Body(&query); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Edu.RenderChallengeLiveDetails(query)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

func (h *RenderHandler) RenderEducationPowerBonus(c fiber.Ctx) error {
	var req drawing.PowerBonusDetailRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Edu.RenderPowerBonusDetail(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.AreaItemUpgradeMaterialsRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Edu.RenderAreaItemUpgradeMaterials(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.BondsRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Edu.RenderBonds(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.LeaderCountRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.Edu.RenderLeaderCount(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
}

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
	var req rendersk.LineRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderLine(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.SKRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderQuery(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.CFRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderCheckRoom(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.SpeedRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderSpeed(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.PlayerTraceRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderPlayerTrace(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.RankTraceRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderRankTrace(req)
	if err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
	}
	c.Type("png")
	return c.Status(fiber.StatusOK).Send(image)
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
	var req drawing.WinRateRequest
	if err := c.Bind().Body(&req); err != nil {
		return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
	}

	image, err := h.app.SK.RenderWinRate(req)
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
