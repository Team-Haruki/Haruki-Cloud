package pjsk

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	commandhandler "haruki-cloud/internal/pjsk/handler"
)

func stageOperationCount(snapshot commandtrace.Snapshot, name string) int {
	for _, stat := range snapshot.Operations {
		if stat.Name == name {
			return stat.Count
		}
	}
	return 0
}

func TestParseBotRequestStageTimers(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "invalid"}[invalid], func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(t.Context())
			app := fiber.New()
			app.Post("/", func(c fiber.Ctx) error {
				c.SetContext(ctx)
				_, err := parseBotRequest(c)
				if (err != nil) != invalid {
					t.Errorf("parse error = %v", err)
				}
				return c.SendStatus(204)
			})
			body := `{"message":[{"type":"text","data":{"text":"/pjsktzHKT"}}]}`
			if invalid {
				body = "{"
			}
			request := httptest.NewRequest("POST", "/", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			snapshot := trace.Snapshot()
			if got := stageOperationCount(snapshot, "request.body_decode"); got != 1 {
				t.Fatalf("decode count = %d", got)
			}
			wantMessage := 1
			if invalid {
				wantMessage = 0
			}
			for _, name := range []string{"request.message_parse", "request.detach"} {
				if got := stageOperationCount(snapshot, name); got != wantMessage {
					t.Errorf("%s count = %d, want %d", name, got, wantMessage)
				}
			}
			if len(snapshot.Phases) != 1 || snapshot.Phases[0].Name != "request_decode" || snapshot.Phases[0].Count != 1 {
				t.Fatalf("unexpected phases: %+v", snapshot.Phases)
			}
		})
	}
}

func TestResolveBotCommandStageTimers(t *testing.T) {
	commandhandler.EnsureCommandHandlersRegistered()
	ctx, trace := commandtrace.WithTrace(t.Context())
	_, err := resolveBotCommand(ctx, onebot11.Message{onebot11.Text("/pjsktzHKT")}, "profile/timezone", BotCommandRequest{Platform: "qq", PlatformUserID: "12345", MatchedCommand: "/pjsktzHKT"}, testBotID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"command.context_build", "command.match", "command.parse"} {
		if got := stageOperationCount(trace.Snapshot(), name); got != 1 {
			t.Errorf("%s count = %d", name, got)
		}
	}
	ctx, trace = commandtrace.WithTrace(t.Context())
	_, err = resolveBotCommand(ctx, onebot11.Message{onebot11.Text("/unknown-timer-command")}, "unknown", BotCommandRequest{MatchedCommand: "/unknown-timer-command"}, testBotID)
	if err == nil {
		t.Fatal("unknown command should fail")
	}
	if stageOperationCount(trace.Snapshot(), "command.match") != 1 || stageOperationCount(trace.Snapshot(), "command.parse") != 0 {
		t.Fatalf("invalid command statistics: %+v", trace.Snapshot())
	}
}

func TestReplayBypassStillRecordsCheck(t *testing.T) {
	ctx, trace := commandtrace.WithTrace(t.Context())
	var guard *replayGuard
	if !guard.allow(ctx, testBotID, BotCommandRequest{}) {
		t.Fatal("nil guard should allow")
	}
	if stageOperationCount(trace.Snapshot(), "request.replay_check") != 1 {
		t.Fatalf("missing replay operation: %+v", trace.Snapshot())
	}
}

func TestSharedResponseStageTimersUseExecutionContext(t *testing.T) {
	for _, mode := range []string{"success", "encoding-error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(t.Context())
			result := executeSharedCommandWithTrace(ctx, func(executionCtx context.Context) sharedCommandResult {
				if mode == "panic" {
					panic("test panic")
				}
				envelope := newBotResponseEnvelope(200, "ok")
				if mode == "encoding-error" {
					envelope.Data = func() {}
				}
				return encodeSharedCommandResult(executionCtx, envelope, sharedCommandMetadata{}, false)
			}, nil)
			if len(result.Response.JSONBody) == 0 || len(result.Response.MsgPackBody) == 0 {
				t.Fatal("shared response must retain both wire formats")
			}
			if len(trace.Snapshot().Operations) != 0 {
				t.Fatal("shared execution must use its independent trace")
			}
			mergeSharedCommandOperations(ctx, result.Operations)
			wantJSON := 1
			if mode == "encoding-error" {
				wantJSON = 2
			}
			for name, want := range map[string]int{
				"command.shared.operation.response.json_encode":    wantJSON,
				"command.shared.operation.response.msgpack_encode": 1,
				"command.shared.operation.response_payload_encode": 1,
			} {
				if got := stageOperationCount(trace.Snapshot(), name); got != want {
					t.Errorf("%s count = %d, want %d", name, got, want)
				}
			}
			if len(trace.Snapshot().Phases) != 0 {
				t.Fatal("shared execution must not merge exclusive phases into the parent")
			}
		})
	}
}
