package pjsk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

const (
	traceCommandPathKey   = "haruki.command.path"
	traceCommandKey       = "haruki.command.matched"
	traceCommandModeKey   = "haruki.command.mode"
	traceCommandModuleKey = "haruki.command.module"
	traceCommandRegionKey = "haruki.command.region"
	traceOutcomeKey       = "haruki.command.outcome"
	traceErrorTypeKey     = "haruki.command.error_type"
)

var commandTelemetryLogger = logger.NewLoggerWithCommandWriter("Command", "INFO")

func commandTraceMiddleware(c fiber.Ctx) (err error) {
	startedAt := time.Now()
	requestBytes := len(c.Request().Body())
	var observedErr error
	ctx, trace := commandtrace.WithTrace(c.Context())
	requestID := requestid.FromContext(c)
	if requestID == "" {
		requestID = strings.TrimSpace(c.RequestID())
	}
	if requestID == "" {
		requestID = newCommandRequestID()
		c.Set(fiber.HeaderXRequestID, requestID)
	}
	botID := strings.TrimSpace(c.Params("botId"))
	ctx = logger.WithContextAttrs(ctx,
		slog.String("request_id", requestID),
		slog.String("bot_id", botID),
	)
	c.SetContext(ctx)

	defer func() {
		if recovered := recover(); recovered != nil {
			setCommandTraceOutcome(c, "error", nil)
			c.Locals(traceErrorTypeKey, "panic")
			commandtrace.SetErrorType(ctx, "panic")
			attrs := []any{"panic_type", fmt.Sprintf("%T", recovered)}
			if !harukiConfig.Cfg.Profile.IsProduction() {
				attrs = append(attrs, "stack", string(debug.Stack()))
			}
			logger.ErrorContext(ctx, "bot command panic recovered", attrs...)
			observedErr = fiber.ErrInternalServerError
			renderCommandError(c, ctx, observedErr)
			err = nil
		}
		emitCommandTrace(c, ctx, trace, startedAt, requestBytes, observedErr)
	}()
	observedErr = c.Next()
	if observedErr == nil {
		return nil
	}
	commandtrace.SetErrorType(ctx, fmt.Sprintf("%T", observedErr))
	renderCommandError(c, ctx, observedErr)
	return nil
}

func renderCommandError(c fiber.Ctx, ctx context.Context, cause error) {
	finishResponse := commandtrace.MeasurePhase(ctx, "response_encode")
	defer finishResponse()
	if handlerErr := c.App().ErrorHandler(c, cause); handlerErr != nil {
		commandtrace.SetErrorType(ctx, fmt.Sprintf("%T", handlerErr))
		logger.ErrorContext(ctx, "bot command error handler failed", "error_type", fmt.Sprintf("%T", handlerErr))
		c.Response().SetStatusCode(fiber.StatusInternalServerError)
		c.Response().Header.SetContentType("text/plain; charset=utf-8")
		c.Response().SetBodyString("Internal Server Error")
	}
}

func emitCommandTrace(c fiber.Ctx, ctx context.Context, trace *commandtrace.Trace, startedAt time.Time, requestBytes int, err error) {
	serverDuration := time.Since(startedAt)
	if !strings.Contains(c.Path(), "/pjsk/") {
		return
	}

	snapshot := trace.Snapshot()
	statusCode := c.Response().StatusCode()
	outcome := localString(c, traceOutcomeKey)
	switch {
	case statusCode >= fiber.StatusInternalServerError:
		outcome = "error"
	case statusCode >= fiber.StatusBadRequest:
		outcome = "rejected"
	case err != nil:
		outcome = "error"
	case outcome == "":
		outcome = "ok"
	}

	attrs := []any{
		"event", "bot_command",
		"http_method", c.Method(),
		"http_route", c.Route().Path,
		"status_code", statusCode,
		"command", localString(c, traceCommandKey),
		"command_path", localString(c, traceCommandPathKey),
		"command_module", localString(c, traceCommandModuleKey),
		"command_mode", localString(c, traceCommandModeKey),
		"region", localString(c, traceCommandRegionKey),
		"outcome", outcome,
		"server_duration_ms", commandtrace.Milliseconds(serverDuration),
		"duration_scope", "bot_command_middleware",
		"accounted_phase_ms", commandtrace.Milliseconds(snapshot.AccountedPhaseDuration()),
		"unattributed_ms", commandtrace.Milliseconds(snapshot.UnattributedDuration(serverDuration)),
		"phase_overrun_ms", commandtrace.Milliseconds(snapshot.PhaseOverrunDuration(serverDuration)),
		"request_bytes", requestBytes,
		"response_bytes", len(c.Response().Body()),
		"phase_stats_kind", "exclusive",
		"phase_stats", snapshot.PhaseValue(),
		"operation_stats_kind", "inclusive",
		"operation_stats", snapshot.OperationValue(),
	}
	if snapshot.ErrorType != "" {
		attrs = append(attrs, "error_type", snapshot.ErrorType)
	} else if errorType := localString(c, traceErrorTypeKey); errorType != "" {
		attrs = append(attrs, "error_type", errorType)
	} else if err != nil {
		attrs = append(attrs, "error_type", fmt.Sprintf("%T", err))
	}
	commandTelemetryLogger.InfoContext(ctx, "bot command completed", attrs...)
}

func newCommandRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

func setCommandTraceMetadata(c fiber.Ctx, matchedCommand, commandPath string) {
	c.Locals(traceCommandKey, strings.TrimSpace(matchedCommand))
	c.Locals(traceCommandPathKey, strings.TrimSpace(commandPath))
}

func setResolvedCommandTraceMetadata(c fiber.Ctx, module, mode, region string) {
	c.Locals(traceCommandModuleKey, strings.TrimSpace(module))
	c.Locals(traceCommandModeKey, strings.TrimSpace(mode))
	c.Locals(traceCommandRegionKey, safeCommandTraceRegion(region))
}

func safeCommandTraceRegion(region string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(region)); normalized {
	case "":
		return ""
	case "jp", "cn", "tw", "kr", "en":
		return normalized
	default:
		return "unknown"
	}
}

func setCommandTraceOutcome(c fiber.Ctx, outcome string, err error) {
	c.Locals(traceOutcomeKey, strings.TrimSpace(outcome))
	if err != nil {
		c.Locals(traceErrorTypeKey, fmt.Sprintf("%T", err))
	}
}

func localString(c fiber.Ctx, key string) string {
	value, _ := c.Locals(key).(string)
	return strings.TrimSpace(value)
}
