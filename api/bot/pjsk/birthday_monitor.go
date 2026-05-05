package pjsk

import (
	"fmt"
	"strings"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/subscription"
	"haruki-cloud/utils/logger"

	json "github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/shamaton/msgpack/v3"
)

type birthdayMonitorClientAction struct {
	Type                string `json:"type" msgpack:"type"`
	SubscriptionID      string `json:"subscription_id" msgpack:"subscription_id"`
	SubscriptionVersion string `json:"subscription_version" msgpack:"subscription_version"`
	Endpoint            string `json:"endpoint" msgpack:"endpoint"`
	Token               string `json:"token" msgpack:"token"`
	ExpiresAt           int64  `json:"expires_at" msgpack:"expires_at"`
}

type birthdayRenderRequest struct {
	Platform            string `json:"platform" msgpack:"platform"`
	PlatformUserID      string `json:"platform_user_id" msgpack:"platform_user_id"`
	PlatformGroupID     string `json:"platform_group_id" msgpack:"platform_group_id"`
	SelfID              string `json:"self_id" msgpack:"self_id"`
	SubscriptionID      string `json:"subscription_id" msgpack:"subscription_id"`
	SubscriptionVersion string `json:"subscription_version" msgpack:"subscription_version"`
	Token               string `json:"token" msgpack:"token"`
	EventID             string `json:"event_id" msgpack:"event_id"`
}

type activeBirthdaySubscriptionResponse struct {
	Active         bool     `json:"active"`
	SubscriptionID string   `json:"subscription_id,omitempty"`
	Materials      []string `json:"materials,omitempty"`
	MaterialIDs    []int    `json:"material_ids,omitempty"`
	NotifyEmpty    bool     `json:"notify_empty"`
}

type birthdayEventWriteRequest struct {
	SubscriptionID     string         `json:"subscription_id" msgpack:"subscription_id"`
	Region             string         `json:"region" msgpack:"region"`
	UID                string         `json:"uid" msgpack:"uid"`
	UploadTime         int64          `json:"upload_time" msgpack:"upload_time"`
	MatchedMaterialIDs []int          `json:"matched_material_ids" msgpack:"matched_material_ids"`
	EmptyResult        bool           `json:"empty_result" msgpack:"empty_result"`
	FilteredPayload    map[string]any `json:"filtered_payload" msgpack:"filtered_payload"`
}

type birthdayEventWriteResponse struct {
	EventID        string `json:"event_id"`
	SubscriptionID string `json:"subscription_id"`
	EmptyResult    bool   `json:"empty_result"`
}

type birthdayTokenValidationResponse struct {
	Valid               bool                                `json:"valid"`
	SubscriptionID      string                              `json:"subscription_id,omitempty"`
	SubscriptionVersion string                              `json:"subscription_version,omitempty"`
	ExpiresAt           int64                               `json:"expires_at,omitempty"`
	PendingEvents       []subscription.PendingBirthdayEvent `json:"pending_events,omitempty"`
}

const birthdayMonitorCommandPath = "mysekai/birthday-monitor"

var birthdayMonitorCommandPrefixes = []string{
	"/烤森生日取消监听",
	"/mysekai birthday unmonitor",
	"/ms生日取消监听",
	"/烤森生日监听",
	"/mysekai birthday monitor",
	"/ms生日监听",
}

var birthdayMonitorCommandPrefixRegions = []string{"jp", "tw", "kr", "en", "cn"}

var birthdayMonitorManifestCommandPrefixes = buildBirthdayMonitorManifestCommandPrefixes(birthdayMonitorCommandPrefixes)

func registerBirthdayMonitorRoutes(pjsk fiber.Router, app *fiber.App, renderApp *renderapp.App) {
	if renderApp == nil {
		return
	}
	pjsk.Post("/"+birthdayMonitorCommandPath, makeBirthdayMonitorHandler(renderApp))
	pjsk.Post("/"+birthdayMonitorCommandPath+"/render", makeBirthdayMonitorRenderHandler(renderApp))
	pjsk.Post("/"+birthdayMonitorCommandPath+"/ack", makeBirthdayMonitorAckHandler(renderApp))

	internal := app.Group("/internal", api.VerifyAPIAuthorization())
	internal.Get("/subscriptions/mysekai-birthday/active", makeBirthdayMonitorActiveHandler(renderApp))
	internal.Get("/subscriptions/mysekai-birthday/validate", makeBirthdayMonitorTokenValidateHandler(renderApp))
	internal.Post("/subscription-events/mysekai-birthday", makeBirthdayMonitorEventWriteHandler(renderApp))
}

func makeBirthdayMonitorHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		req, err := parseBotRequest(c)
		if err != nil {
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		req.SelfID = strings.TrimSpace(req.SelfID)
		botID := strings.TrimSpace(c.Params("botId"))
		service := subscription.NewServiceWithToolbox(renderApp.PJSK, renderApp.Bindings, renderApp.Toolbox)
		text := birthdayMonitorCommandText(req)
		regionExplicit := strings.TrimSpace(req.Server) != ""

		if isCancelBirthdayMonitorText(text) {
			if _, err := service.Cancel(c.Context(), req.Platform, req.PlatformUserID, req.PlatformGroupID, botID, req.SelfID, req.Server, regionExplicit, text); err != nil {
				logger.Warnf("birthday monitor cancel failed: bot_id=%s user=%s err=%v", botID, req.PlatformUserID, err)
				return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(err.Error())})
			}
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text("烤森生日材料监听已取消。")})
		}

		result, err := service.CreateOrUpdate(c.Context(), req.Platform, req.PlatformUserID, req.PlatformGroupID, botID, req.SelfID, req.Server, regionExplicit, text, req.NotifyEmpty)
		if err != nil {
			logger.Warnf("birthday monitor upsert failed: bot_id=%s user=%s err=%v", botID, req.PlatformUserID, err)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(err.Error())})
		}

		visible := onebot11.Message{onebot11.Text(fmt.Sprintf("烤森生日材料监听已更新，有效期 %d 分钟。", int(result.Duration.Minutes())))}
		actions := birthdayMonitorActions(result)
		return botResponseWithActions(c, fiber.StatusOK, api.ResponseOK, visible, actions)
	}
}

func makeBirthdayMonitorRenderHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		var req birthdayRenderRequest
		if err := parseRequestBody(c, &req); err != nil {
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		botID := strings.TrimSpace(c.Params("botId"))
		service := subscription.NewServiceWithToolbox(renderApp.PJSK, renderApp.Bindings, renderApp.Toolbox)
		event, err := service.EventForClient(c.Context(), req.EventID, req.SubscriptionID, req.SubscriptionVersion, req.Token, botID, req.PlatformGroupID, req.PlatformUserID, req.SelfID)
		if err != nil {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(err.Error())})
		}
		if event.EmptyResult {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{
				onebot11.At(event.PlatformUserID),
				onebot11.Text(subscription.EmptyBirthdayMonitorMessage),
			})
		}
		if len(event.FilteredPayload) == 0 {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text("订阅事件缺少可绘制数据")})
		}
		if renderApp.MySekai == nil || renderApp.ImageCache == nil {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text("烤森服务未就绪，请稍后再试")})
		}
		data, err := renderApp.MySekai.WithContext(c.Context()).WithMySekaiData(event.FilteredPayload).RenderMap(rendermysekai.MapQuery{Region: event.Region})
		if err != nil {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(err.Error())})
		}
		url, err := renderApp.ImageCache.StoreAndGetURL(c.Context(), data, "pjsk")
		if err != nil {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(err.Error())})
		}
		return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{
			onebot11.At(event.PlatformUserID),
			onebot11.Image(url, ""),
		})
	}
}

func makeBirthdayMonitorAckHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		var req birthdayRenderRequest
		if err := parseRequestBody(c, &req); err != nil {
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		botID := strings.TrimSpace(c.Params("botId"))
		service := subscription.NewServiceWithToolbox(renderApp.PJSK, renderApp.Bindings, renderApp.Toolbox)
		if err := service.AckEvent(c.Context(), req.EventID, req.SubscriptionID, req.SubscriptionVersion, req.Token, botID, req.PlatformGroupID, req.PlatformUserID, req.SelfID); err != nil {
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(err.Error())})
		}
		return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
	}
}

func makeBirthdayMonitorActiveHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		service := subscription.NewServiceWithToolbox(renderApp.PJSK, renderApp.Bindings, renderApp.Toolbox)
		result, err := service.ActiveForUpload(c.Context(), c.Query("region"), c.Query("uid"))
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusOK).JSON(activeBirthdaySubscriptionResponse{
			Active:         result.Active,
			SubscriptionID: result.SubscriptionID,
			Materials:      result.Materials,
			MaterialIDs:    result.MaterialIDs,
			NotifyEmpty:    result.NotifyEmpty,
		})
	}
}

func makeBirthdayMonitorTokenValidateHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		service := subscription.NewServiceWithToolbox(renderApp.PJSK, renderApp.Bindings, renderApp.Toolbox)
		result, err := service.ValidateToken(c.Context(), c.Query("subscription_id"), c.Query("subscription_version"), c.Query("token"))
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, err.Error())
		}
		resp := birthdayTokenValidationResponse{Valid: result.Valid}
		if result.Valid && result.Subscription != nil {
			resp.SubscriptionID = fmt.Sprint(result.Subscription.ID)
			resp.SubscriptionVersion = result.SubscriptionVersion
			resp.ExpiresAt = result.Subscription.ExpiresAt.Unix()
			resp.PendingEvents = result.PendingEvents
		}
		return c.Status(fiber.StatusOK).JSON(resp)
	}
}

func makeBirthdayMonitorEventWriteHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		var req birthdayEventWriteRequest
		if err := parseRequestBody(c, &req); err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		payload, err := json.Marshal(req.FilteredPayload)
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, "invalid filtered_payload")
		}
		service := subscription.NewService(renderApp.PJSK, renderApp.Bindings)
		stored, err := service.StoreEvent(c.Context(), subscription.BirthdayEventPayload{
			SubscriptionID:     req.SubscriptionID,
			Region:             req.Region,
			UID:                req.UID,
			UploadTime:         req.UploadTime,
			MatchedMaterialIDs: req.MatchedMaterialIDs,
			EmptyResult:        req.EmptyResult,
			FilteredPayload:    payload,
		})
		if err != nil {
			return api.JSONResponse(c, fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusOK).JSON(birthdayEventWriteResponse{
			EventID:        stored.EventID,
			SubscriptionID: stored.SubscriptionID,
			EmptyResult:    stored.EmptyResult,
		})
	}
}

func birthdayMonitorActions(result *subscription.BirthdayMonitorResult) []birthdayMonitorClientAction {
	base := strings.TrimRight(strings.TrimSpace(harukiConfig.Cfg.HMES.PublicBaseURL), "/")
	if result == nil || result.Subscription == nil || base == "" {
		return nil
	}
	return []birthdayMonitorClientAction{{
		Type:                "hmes_sse",
		SubscriptionID:      fmt.Sprint(result.Subscription.ID),
		SubscriptionVersion: result.SubscriptionVersion,
		Endpoint:            base + "/sse",
		Token:               result.Token,
		ExpiresAt:           result.Subscription.ExpiresAt.Unix(),
	}}
}

func botResponseWithActions(c fiber.Ctx, status int, message string, data any, actions []birthdayMonitorClientAction) error {
	resp := fiber.Map{
		"status":         status,
		"message":        message,
		"data":           data,
		"client_actions": actions,
	}
	if c.Locals("secure_noise") == nil {
		return c.Status(status).JSON(resp)
	}
	encoded, err := msgpack.Marshal(resp)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Set("Content-Type", api.ContentTypeMsgPack)
	return c.Status(status).Send(encoded)
}

func parseRequestBody(c fiber.Ctx, out any) error {
	ct := string(c.Request().Header.ContentType())
	if strings.Contains(ct, "msgpack") {
		return msgpack.Unmarshal(c.Body(), out)
	}
	return c.Bind().Body(out)
}

func requestMessageText(req BotCommandRequest) string {
	var builder strings.Builder
	for _, segment := range req.Message {
		if segment.Type != onebot11.TypeText {
			continue
		}
		switch data := segment.Data.(type) {
		case onebot11.TextData:
			builder.WriteString(data.Text)
		case map[string]string:
			builder.WriteString(data[onebot11.KeyText])
		case map[string]any:
			if text, _ := data[onebot11.KeyText].(string); text != "" {
				builder.WriteString(text)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func birthdayMonitorCommandText(req BotCommandRequest) string {
	text := requestMessageText(req)
	if _, err := subscription.ParseBirthdayMonitorCommand(text); err == nil {
		return text
	}

	matchedCommand := strings.TrimSpace(req.MatchedCommand)
	if matchedCommand == "" {
		return text
	}
	if text == "" {
		return matchedCommand
	}
	return strings.TrimSpace(matchedCommand + " " + text)
}

func buildBirthdayMonitorManifestCommandPrefixes(commands []string) []string {
	result := make([]string, 0, len(commands)*(len(birthdayMonitorCommandPrefixRegions)+1))
	seen := make(map[string]struct{}, len(commands))
	add := func(command string) {
		command = strings.TrimSpace(command)
		if command == "" {
			return
		}
		if _, ok := seen[command]; ok {
			return
		}
		seen[command] = struct{}{}
		result = append(result, command)
	}
	for _, command := range commands {
		add(command)
		for _, region := range birthdayMonitorCommandPrefixRegions {
			add("/" + region + strings.TrimPrefix(command, "/"))
		}
	}
	return result
}

func isCancelBirthdayMonitorText(text string) bool {
	parsed, err := subscription.ParseBirthdayMonitorCommand(text)
	return err == nil && parsed.Cancel
}
