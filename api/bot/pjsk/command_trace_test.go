package pjsk

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/core/crypto"
	"haruki-cloud/internal/middleware/secure"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

func TestCommandTraceMiddlewareEmitsOneStructuredSummary(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	app := fiber.New()
	app.Use(requestid.New())
	app.Post("/api/v2/bot/:botId/pjsk/test", commandTraceMiddleware, func(c fiber.Ctx) error {
		c.Request().SetBody([]byte("decrypted-request-body"))
		setCommandTraceMetadata(c, "/test", "test")
		setResolvedCommandTraceMetadata(c, "card", "box", "jp")
		setCommandTraceOutcome(c, "ok", nil)
		commandtrace.RecordPhase(c.Context(), "request_decode", 2*time.Millisecond)
		commandtrace.RecordOperation(c.Context(), "asset.stat", 375*time.Microsecond)
		return c.SendString("ok")
	})

	response, err := app.Test(httptest.NewRequest("POST", "/api/v2/bot/66666666/pjsk/test", strings.NewReader("{}")))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("command summaries = %d, output=%q", len(lines), output.String())
	}
	line := lines[0]
	for _, field := range []string{
		"component=Command",
		"event=bot_command",
		"bot_id=66666666",
		"command=/test",
		"command_path=test",
		"command_module=card",
		"command_mode=box",
		"region=jp",
		"outcome=ok",
		"server_duration_ms=",
		"duration_scope=bot_command_middleware",
		"request_bytes=2",
		"unattributed_ms=",
		"phase_stats.request_decode.count=1",
		"phase_stats.request_decode.total_ms=2",
		"operation_stats.asset.stat.count=1",
		"operation_stats.asset.stat.total_ms=0.375",
	} {
		if !strings.Contains(line, field) {
			t.Errorf("summary missing %q: %s", field, line)
		}
	}
}

func TestCommandTraceMiddlewareEmitsSummaryAfterPanicRecovery(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	app := fiber.New()
	app.Use(requestid.New())
	app.Use(recover.New())
	app.Post("/api/v2/bot/:botId/pjsk/test", commandTraceMiddleware, func(c fiber.Ctx) error {
		panic("boom")
	})

	response, err := app.Test(httptest.NewRequest("POST", "/api/v2/bot/1/pjsk/test", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d", response.StatusCode)
	}
	line := output.String()
	if strings.Count(line, "event=bot_command") != 1 {
		t.Fatalf("expected one command summary: %q", line)
	}
	if !strings.Contains(line, "outcome=error") || !strings.Contains(line, "error_type=panic") {
		t.Fatalf("panic metadata missing: %q", line)
	}
	if !strings.Contains(line, "phase_stats.response_encode.count=1") || !strings.Contains(line, "response_bytes=") {
		t.Fatalf("panic response timing missing: %q", line)
	}
}

func TestCommandTraceMiddlewareClassifiesReturned4xxAsRejected(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	app := fiber.New()
	app.Post("/api/v2/bot/:botId/pjsk/test", commandTraceMiddleware, func(c fiber.Ctx) error {
		return fiber.ErrBadRequest
	})

	response, err := app.Test(httptest.NewRequest("POST", "/api/v2/bot/1/pjsk/test", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
	line := output.String()
	for _, field := range []string{
		"outcome=rejected",
		"error_type=*fiber.Error",
		"phase_stats.response_encode.count=1",
	} {
		if !strings.Contains(line, field) {
			t.Fatalf("4xx summary missing %q: %q", field, line)
		}
	}
}

func TestCommandTraceMiddlewareEncryptsSecureFailuresBeforeSummary(t *testing.T) {
	tests := []struct {
		name          string
		handler       fiber.Handler
		wantStatus    int
		wantBody      string
		wantOutcome   string
		wantErrorType string
	}{
		{
			name: "returned error",
			handler: func(c fiber.Ctx) error {
				return fiber.ErrBadRequest
			},
			wantStatus:    fiber.StatusBadRequest,
			wantBody:      fiber.ErrBadRequest.Message,
			wantOutcome:   "rejected",
			wantErrorType: "*fiber.Error",
		},
		{
			name: "panic",
			handler: func(c fiber.Ctx) error {
				panic("boom")
			},
			wantStatus:    fiber.StatusInternalServerError,
			wantBody:      fiber.ErrInternalServerError.Message,
			wantOutcome:   "error",
			wantErrorType: "panic",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger.SetGlobalFileWriter(&output)
			t.Cleanup(func() { logger.SetGlobalFileWriter(os.Stdout) })

			serverKey, err := crypto.GenerateKeyPair()
			if err != nil {
				t.Fatalf("generate server key: %v", err)
			}
			initiator, err := crypto.NewInitiator(serverKey.Public)
			if err != nil {
				t.Fatalf("create initiator: %v", err)
			}
			ciphertext, err := initiator.EncryptPacket([]byte(`{"request":"test"}`))
			if err != nil {
				t.Fatalf("encrypt request: %v", err)
			}

			app := fiber.New()
			app.Use(requestid.New())
			app.Use(recover.New())
			app.Post(
				"/api/v2/bot/:botId/pjsk/test",
				commandTraceMiddleware,
				secure.New(secure.Config{ServerPrivateKey: serverKey}),
				test.handler,
			)

			response, err := app.Test(httptest.NewRequest("POST", "/api/v2/bot/1/pjsk/test", bytes.NewReader(ciphertext)))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.wantStatus)
			}
			encryptedResponse, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			plaintext, err := initiator.DecryptPacket(encryptedResponse)
			if err != nil {
				t.Fatalf("decrypt response: %v", err)
			}
			if string(plaintext) != test.wantBody {
				t.Fatalf("response = %q, want %q", plaintext, test.wantBody)
			}

			line := output.String()
			for _, field := range []string{
				"event=bot_command",
				"outcome=" + test.wantOutcome,
				"error_type=" + test.wantErrorType,
				"phase_stats.response_encode.count=1",
				"phase_stats.noise_encrypt.count=1",
				fmt.Sprintf("response_bytes=%d", len(encryptedResponse)),
			} {
				if !strings.Contains(line, field) {
					t.Fatalf("secure summary missing %q: %q", field, line)
				}
			}
			if strings.Count(line, "event=bot_command") != 1 {
				t.Fatalf("command summaries = %d: %q", strings.Count(line, "event=bot_command"), line)
			}
		})
	}
}

func TestCommandTraceMiddlewareUsesFinalHTTPStatusForOutcome(t *testing.T) {
	var output bytes.Buffer
	logger.SetGlobalFileWriter(&output)
	defer logger.SetGlobalFileWriter(os.Stdout)

	app := fiber.New()
	app.Post("/api/v2/bot/:botId/pjsk/test", commandTraceMiddleware, func(c fiber.Ctx) error {
		setCommandTraceOutcome(c, "ok", nil)
		return c.Status(fiber.StatusUnprocessableEntity).SendString("invalid")
	})

	response, err := app.Test(httptest.NewRequest("POST", "/api/v2/bot/1/pjsk/test", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if response.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if line := output.String(); !strings.Contains(line, "outcome=rejected") {
		t.Fatalf("final status did not override optimistic outcome: %q", line)
	}
}

func TestCommandTraceLabelsRejectUntrustedCommands(t *testing.T) {
	allowed := []string{"/card", "/card box"}
	if got := allowedCommandTraceLabel("/card box", allowed); got != "/card box" {
		t.Fatalf("allowed label = %q", got)
	}
	if got := allowedCommandTraceLabel("uid=123456", allowed); got != "<invalid>" {
		t.Fatalf("untrusted label = %q", got)
	}
}

func TestReplayErrorIsExpectedCommandRejection(t *testing.T) {
	if !isExpectedCommandError(onebot11.NewReplayError("invalid query")) {
		t.Fatal("ReplayError should be classified as a rejection")
	}
	if isExpectedCommandError(fmt.Errorf("database unavailable")) {
		t.Fatal("unexpected internal error should remain an error")
	}
}

func TestSafeCommandTraceRegionRejectsUntrustedValues(t *testing.T) {
	for input, want := range map[string]string{
		" JP ":                 "jp",
		"cn":                   "cn",
		"https://secret.local": "unknown",
		"arbitrary-user-value": "unknown",
		"":                     "",
	} {
		if got := safeCommandTraceRegion(input); got != want {
			t.Fatalf("safeCommandTraceRegion(%q) = %q, want %q", input, got, want)
		}
	}
}
