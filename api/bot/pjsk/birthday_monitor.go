package pjsk

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/api"
	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	pjskhandler "haruki-cloud/internal/pjsk/handler"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/subscription"
	"haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/shamaton/msgpack/v3"
	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/utils/usererror"
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

const birthdayMonitorCommandPath = pjskhandler.BirthdayMonitorHelpPath

var birthdayMonitorCommandPrefixes = []string{
	"/烤森生日取消监听", //copylint:ignore 指令触发词
	"/mysekai birthday unmonitor",
	"/ms生日取消监听", //copylint:ignore 指令触发词
	"/烤森生日监听",   //copylint:ignore 指令触发词
	"/mysekai birthday monitor",
	"/ms生日监听", //copylint:ignore 指令触发词
}

var birthdayMonitorCommandPrefixRegions = []string{"jp", "tw", "kr", "en", "cn"}

var birthdayMonitorManifestCommandPrefixes = buildBirthdayMonitorManifestCommandPrefixes(birthdayMonitorCommandPrefixes)

func registerBirthdayMonitorRoutes(pjsk fiber.Router, app *fiber.App, renderApp *renderapp.App, guard commandRequestGuard) {
	if renderApp == nil {
		return
	}
	pjsk.Post("/"+birthdayMonitorCommandPath, makeBirthdayMonitorHandler(renderApp, guard))
	pjsk.Post("/"+birthdayMonitorCommandPath+"/render", makeBirthdayMonitorRenderHandler(renderApp))
	pjsk.Post("/"+birthdayMonitorCommandPath+"/ack", makeBirthdayMonitorAckHandler(renderApp))

	internal := app.Group("/internal", api.VerifyAPIAuthorization())
	internal.Get("/subscriptions/mysekai-birthday/active", makeBirthdayMonitorActiveHandler(renderApp))
	internal.Get("/subscriptions/mysekai-birthday/validate", makeBirthdayMonitorTokenValidateHandler(renderApp))
	internal.Post("/subscription-events/mysekai-birthday", makeBirthdayMonitorEventWriteHandler(renderApp))
}

func newBirthdayMonitorService(renderApp *renderapp.App) *subscription.Service {
	if renderApp == nil {
		return nil
	}
	service := subscription.NewServiceWithToolbox(renderApp.PJSK, renderApp.Bindings, renderApp.Toolbox)
	service.SetReadOnly(renderApp.Config.ReadOnly)
	service.SetEventStreams(renderApp.EventStreams)
	return service
}

func newBirthdayMonitorDBService(renderApp *renderapp.App) *subscription.Service {
	if renderApp == nil {
		return nil
	}
	service := subscription.NewService(renderApp.PJSK, renderApp.Bindings)
	service.SetReadOnly(renderApp.Config.ReadOnly)
	return service
}

func makeBirthdayMonitorHandler(renderApp *renderapp.App, guard commandRequestGuard) fiber.Handler {
	return func(c fiber.Ctx) error {
		setCommandTraceMetadata(c, "", birthdayMonitorCommandPath)
		req, err := parseBotRequest(c)
		if err != nil {
			setCommandTraceOutcome(c, "rejected", err)
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		requestCtx := i18n.WithParamEcho(c.Context(), req.EnableParamEcho)
		traceCommand := allowedCommandTraceLabel(req.MatchedCommand, birthdayMonitorManifestCommandPrefixes)
		setCommandTraceMetadata(c, traceCommand, birthdayMonitorCommandPath)
		finishValidation := commandtrace.MeasurePhase(requestCtx, "request_validate")
		req.SelfID = strings.TrimSpace(req.SelfID)
		text := birthdayMonitorCommandText(req)
		regionExplicit := strings.TrimSpace(req.Server) != ""
		finishValidation()
		setResolvedCommandTraceMetadata(c, "pjsk", "birthday_monitor", req.Server)

		finishGuard := commandtrace.MeasurePhase(requestCtx, "request_guard")
		guardLease := acquireRequestGuard(requestCtx, guard, req)
		finishGuard()
		if !guardLease.proceed {
			setCommandTraceOutcome(c, "deduplicated", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
		}
		defer func() {
			finishGuardComplete := commandtrace.MeasurePhase(requestCtx, "guard_complete")
			markRequestGuardComplete(requestCtx, guard, req, guardLease)
			finishGuardComplete()
		}()

		botID := strings.TrimSpace(c.Params("botId"))
		service := newBirthdayMonitorService(renderApp)
		finishExecute := commandtrace.MeasurePhase(requestCtx, "command_execute")
		defer finishPhaseOnPanic(finishExecute)
		helpCommand := birthdayMonitorHelpCommand(text)

		if isBirthdayMonitorHelpText(text) {
			message, err := pjskhandler.RouteHelpMessage(requestCtx, birthdayMonitorCommandPath, renderApp)
			finishExecute()
			if err != nil {
				setCommandTraceOutcome(c, "error", err)
				return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorReply(requestCtx, err, birthdayMonitorCommandPath, helpCommand))})
			}
			setCommandTraceOutcome(c, "ok", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, message)
		}

		if isCancelBirthdayMonitorText(text) {
			_, err := service.Cancel(requestCtx, req.Platform, req.PlatformUserID, req.PlatformGroupID, botID, req.SelfID, req.Server, regionExplicit, text)
			finishExecute()
			if err != nil {
				setCommandTraceOutcome(c, "error", err)
				logger.WarnContext(requestCtx, "birthday monitor cancel failed",
					"command_path", birthdayMonitorCommandPath,
					"command", traceCommand,
					"error_type", fmt.Sprintf("%T", err),
				)
				return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorReply(requestCtx, err, birthdayMonitorCommandPath, helpCommand))})
			}
			setCommandTraceOutcome(c, "ok", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(i18n.T("subscription.birthday.cancelled"))})
		}

		result, err := service.CreateOrUpdate(requestCtx, req.Platform, req.PlatformUserID, req.PlatformGroupID, botID, req.SelfID, req.Server, regionExplicit, text, req.NotifyEmpty)
		finishExecute()
		if err != nil {
			setCommandTraceOutcome(c, "error", err)
			logger.WarnContext(requestCtx, "birthday monitor upsert failed",
				"command_path", birthdayMonitorCommandPath,
				"command", traceCommand,
				"error_type", fmt.Sprintf("%T", err),
			)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorReply(requestCtx, err, birthdayMonitorCommandPath, helpCommand))})
		}

		visible := onebot11.Message{onebot11.Text(i18n.T("subscription.birthday.updated", i18n.Data{"Minutes": int(result.Duration.Minutes())}))}
		actions := birthdayMonitorActions(result)
		setCommandTraceOutcome(c, "ok", nil)
		return botResponseWithActions(c, fiber.StatusOK, api.ResponseOK, visible, actions)
	}
}

// errBirthdayRenderNotConfigured is the logged cause when the birthday
// monitor cannot render: no MySekai renderer or no image host.
var errBirthdayRenderNotConfigured = errors.New("birthday monitor: mysekai renderer or image hosting is not configured")

func makeBirthdayMonitorRenderHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		const commandPath = birthdayMonitorCommandPath + "/render"
		setCommandTraceMetadata(c, "birthday-monitor/render", commandPath)
		setResolvedCommandTraceMetadata(c, "pjsk", "birthday_monitor_render", "")
		var req birthdayRenderRequest
		if err := parseRequestBody(c, &req); err != nil {
			setCommandTraceOutcome(c, "rejected", err)
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		finishValidation := commandtrace.MeasurePhase(c.Context(), "request_validate")
		botID := strings.TrimSpace(c.Params("botId"))
		finishValidation()
		service := newBirthdayMonitorService(renderApp)
		finishExecute := commandtrace.MeasurePhase(c.Context(), "command_execute")
		defer finishPhaseOnPanic(finishExecute)
		event, err := service.EventForClient(c.Context(), req.EventID, req.SubscriptionID, req.SubscriptionVersion, req.Token, botID, req.PlatformGroupID, req.PlatformUserID, req.SelfID)
		if err != nil {
			finishExecute()
			setCommandTraceOutcome(c, "error", err)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorText(c.Context(), err, birthdayMonitorCommandPath, ""))})
		}
		setResolvedCommandTraceMetadata(c, "pjsk", "birthday_monitor_render", event.Region)
		if event.EmptyResult {
			finishExecute()
			setCommandTraceOutcome(c, "ok", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{
				onebot11.At(event.PlatformUserID),
				onebot11.Text(subscription.EmptyBirthdayMonitorMessage),
			})
		}
		if len(event.FilteredPayload) == 0 {
			finishExecute()
			setCommandTraceOutcome(c, "rejected", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorText(c.Context(), usererror.New(usererror.CodeNotFound, i18n.M("subscription.birthday.event_no_data")), birthdayMonitorCommandPath, ""))})
		}
		if renderApp.MySekai == nil || (renderApp.ImageCache == nil && renderApp.ImageHosts.Len() == 0) {
			finishExecute()
			setCommandTraceOutcome(c, "error", nil)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorText(c.Context(), usererror.Misconfigured(errBirthdayRenderNotConfigured), birthdayMonitorCommandPath, ""))})
		}
		data, err := renderApp.MySekai.WithContext(c.Context()).WithMySekaiData(event.FilteredPayload).RenderMapImage(rendermysekai.MapQuery{
			Region: event.Region,
			// Battery and amethyst are not rare; outline their harvest
			// points so every subscribed material stands out.
			HighlightMaterialIDs: subscription.AllMaterialIDs(),
		})
		if err != nil {
			finishExecute()
			setCommandTraceOutcome(c, "error", err)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorText(c.Context(), err, birthdayMonitorCommandPath, ""))})
		}
		renderContext := &pjskhandler.RequestContext{Ctx: c.Context(), App: renderApp}
		images, err := renderContext.RenderedImageMessage(data)
		finishExecute()
		if err != nil {
			setCommandTraceOutcome(c, "error", err)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorText(c.Context(), err, birthdayMonitorCommandPath, ""))})
		}
		setCommandTraceOutcome(c, "ok", nil)
		return botResponse(c, fiber.StatusOK, api.ResponseOK, append(onebot11.Message{onebot11.At(event.PlatformUserID)}, images...))
	}
}

func makeBirthdayMonitorAckHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		const commandPath = birthdayMonitorCommandPath + "/ack"
		setCommandTraceMetadata(c, "birthday-monitor/ack", commandPath)
		setResolvedCommandTraceMetadata(c, "pjsk", "birthday_monitor_ack", "")
		var req birthdayRenderRequest
		if err := parseRequestBody(c, &req); err != nil {
			setCommandTraceOutcome(c, "rejected", err)
			return botResponse(c, fiber.StatusBadRequest, api.ErrInvalidRequest)
		}
		finishValidation := commandtrace.MeasurePhase(c.Context(), "request_validate")
		botID := strings.TrimSpace(c.Params("botId"))
		finishValidation()
		service := newBirthdayMonitorService(renderApp)
		finishExecute := commandtrace.MeasurePhase(c.Context(), "command_execute")
		defer finishPhaseOnPanic(finishExecute)
		if err := service.AckEvent(c.Context(), req.EventID, req.SubscriptionID, req.SubscriptionVersion, req.Token, botID, req.PlatformGroupID, req.PlatformUserID, req.SelfID); err != nil {
			finishExecute()
			setCommandTraceOutcome(c, "error", err)
			return botResponse(c, fiber.StatusOK, api.ResponseOK, onebot11.Message{onebot11.Text(commandErrorText(c.Context(), err, birthdayMonitorCommandPath, ""))})
		}
		finishExecute()
		setCommandTraceOutcome(c, "ok", nil)
		return botResponse(c, fiber.StatusOK, api.ResponseOK, make(onebot11.Message, 0))
	}
}

func finishPhaseOnPanic(finish func()) {
	if recovered := recover(); recovered != nil {
		finish()
		panic(recovered)
	}
}

func makeBirthdayMonitorActiveHandler(renderApp *renderapp.App) fiber.Handler {
	return func(c fiber.Ctx) error {
		service := newBirthdayMonitorService(renderApp)
		result, err := service.ActiveForUpload(c.Context(), c.Query("region"), c.Query("uid"))
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, "failed to query birthday monitor status")
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
		service := newBirthdayMonitorService(renderApp)
		result, err := service.ValidateToken(c.Context(), c.Query("subscription_id"), c.Query("subscription_version"), c.Query("token"))
		if err != nil {
			return api.JSONResponse(c, fiber.StatusInternalServerError, "failed to validate birthday monitor token")
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
		service := newBirthdayMonitorDBService(renderApp)
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
			return api.JSONResponse(c, fiber.StatusBadRequest, "failed to store birthday monitor event")
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
	finish := commandtrace.MeasurePhase(c.Context(), "response_encode")
	defer finish()
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
	finish := commandtrace.MeasurePhase(c.Context(), "request_decode")
	defer finish()
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

// isBirthdayMonitorHelpText reports whether text asks for the monitor's
// help: the command followed by "-help" or "-h".
func isBirthdayMonitorHelpText(text string) bool {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return false
	}
	last := strings.ToLower(fields[len(fields)-1])
	return last == "-help" || last == "-h"
}

// birthdayMonitorHelpCommand is the monitor command text starts with, as
// typed (region prefix included), for the help pointer of usage errors.
func birthdayMonitorHelpCommand(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	best := ""
	for _, command := range birthdayMonitorManifestCommandPrefixes {
		if len(command) > len(best) && strings.HasPrefix(strings.ToLower(text), strings.ToLower(command)) {
			best = command
		}
	}
	return best
}

func isCancelBirthdayMonitorText(text string) bool {
	parsed, err := subscription.ParseBirthdayMonitorCommand(text)
	return err == nil && parsed.Cancel
}
