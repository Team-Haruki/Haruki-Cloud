package pjsk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/shamaton/msgpack/v3"
)

func TestFailedSharedBotCommandLogsRedactedErrorMessage(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	cause := errors.New(`Get "http://192.0.2.10:8080/api/private/game-data/cn/suite/7487590788965145370": connection reset by peer`)
	err := fmt.Errorf("query deck event 181 failed: %w", cause)
	result := failedSharedBotCommand(context.Background(), err, "deck/event", "/组卡", "/组卡",
		sharedCommandMetadata{Outcome: "error", Region: "cn"}, false, "execution")

	wantMessage := `query deck event 181 failed: Get "<url>": connection reset by peer`
	testutil.Require(t, result.Metadata.ErrorMessage == wantMessage, "metadata error message = %q", result.Metadata.ErrorMessage)
	testutil.Require(t, result.Metadata.ErrorType == "*fmt.wrapError", "error type = %q", result.Metadata.ErrorType)

	line := output.String()
	for _, field := range []string{
		`msg="bot command execution failed"`,
		"command_path=deck/event",
		"region=cn",
		"error_type=*fmt.wrapError",
		`error_chain="*fmt.wrapError > *errors.errorString"`,
		`error_message="query deck event 181 failed: Get \"<url>\": connection reset by peer"`,
	} {
		testutil.Check(t, strings.Contains(line, field), "log missing %q: %s", field, line)
	}
	for _, leaked := range []string{"192.0.2.10", "7487590788965145370", "8080"} {
		testutil.Check(t, !strings.Contains(line, leaked), "log leaks %q: %s", leaked, line)
	}
}

func TestFailedSharedBotCommandKeepsRejectionsOutOfErrorLog(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	result := failedSharedBotCommand(context.Background(), notfound.Card("12345678"), "card/detail", "/card", "/card",
		sharedCommandMetadata{Outcome: "error"}, false, "execution")

	testutil.Require(t, result.Metadata.Outcome == "rejected", "outcome = %q", result.Metadata.Outcome)
	testutil.Require(t, result.Metadata.ErrorMessage == "", "rejection carries error message %q", result.Metadata.ErrorMessage)
	testutil.Require(t, !strings.Contains(output.String(), "failed"), "rejection logged as failure: %s", output.String())
}

func TestErrorTypeChain(t *testing.T) {
	base := errors.New("base")
	joined := errors.Join(fmt.Errorf("first: %w", base), errors.New("second"))
	testutil.Require(t, errorTypeChain(nil) == "", "nil chain")
	testutil.Require(t, errorTypeChain(base) == "*errors.errorString", "chain = %q", errorTypeChain(base))
	got := errorTypeChain(fmt.Errorf("outer: %w", joined))
	testutil.Require(t, got == "*fmt.wrapError > *errors.joinError > *fmt.wrapError > *errors.errorString", "chain = %q", got)
	testutil.Require(t, errorTypeChain(errors.Join()) == "", "empty join = %q", errorTypeChain(errors.Join()))

	var deep error = base
	for range maxErrorChainDepth + 3 {
		deep = fmt.Errorf("wrap: %w", deep)
	}
	testutil.Require(t, strings.Count(errorTypeChain(deep), ">") == maxErrorChainDepth-1, "depth not bounded: %q", errorTypeChain(deep))
}

func TestCommandTraceSummaryCarriesErrorMessage(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	app := fiber.New()
	app.Use(requestid.New())
	app.Post("/api/v2/bot/:botId/pjsk/test", commandTraceMiddleware, func(c fiber.Ctx) error {
		applySharedBotCommandMetadata(c, sharedCommandMetadata{
			Command:      "/组卡",
			CommandPath:  "deck/event",
			Region:       "cn",
			Outcome:      "error",
			ErrorType:    "*errors.errorString",
			ErrorMessage: "snapshot: suite snapshot is empty",
		})
		return c.SendString("ok")
	})

	response, err := app.Test(httptest.NewRequest("POST", "/api/v2/bot/66666666/pjsk/test", strings.NewReader("{}")))
	testutil.Require(t, err == nil, "request failed: %v", err)
	testutil.Require(t, response.StatusCode == fiber.StatusOK, "status = %d", response.StatusCode)
	line := output.String()
	testutil.Check(t, strings.Contains(line, "event=bot_command"), "summary missing: %s", line)
	testutil.Check(t, strings.Contains(line, `error_message="snapshot: suite snapshot is empty"`), "summary missing error_message: %s", line)
}

// A result encoded by a node with ErrorMessage decodes on a node without it,
// and the other way round, so a rolling deploy keeps response elections working.
func TestSharedCommandMetadataErrorMessageIsWireCompatible(t *testing.T) {
	type legacyMetadata struct {
		Command       string `msgpack:"command"`
		CommandPath   string `msgpack:"command_path"`
		Module        string `msgpack:"module"`
		Mode          string `msgpack:"mode"`
		Region        string `msgpack:"region"`
		Outcome       string `msgpack:"outcome"`
		ErrorType     string `msgpack:"error_type"`
		ExecutorBotID string `msgpack:"executor_bot_id"`
	}
	current := sharedCommandMetadata{CommandPath: "deck/event", Outcome: "error", ErrorType: "*errors.errorString", ErrorMessage: "boom"}
	encoded, err := msgpack.Marshal(current)
	testutil.Require(t, err == nil, "marshal: %v", err)
	var legacy legacyMetadata
	testutil.Require(t, msgpack.Unmarshal(encoded, &legacy) == nil, "legacy decode failed")
	testutil.Require(t, legacy.CommandPath == "deck/event" && legacy.ErrorType == "*errors.errorString", "legacy = %+v", legacy)

	encoded, err = msgpack.Marshal(legacyMetadata{CommandPath: "card/box", Outcome: "ok"})
	testutil.Require(t, err == nil, "marshal legacy: %v", err)
	var decoded sharedCommandMetadata
	testutil.Require(t, msgpack.Unmarshal(encoded, &decoded) == nil, "current decode failed")
	testutil.Require(t, decoded.CommandPath == "card/box" && decoded.ErrorMessage == "", "decoded = %+v", decoded)
}
