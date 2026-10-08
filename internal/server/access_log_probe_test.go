package server

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	harukiConfig "haruki-cloud/config"
	harukiLogger "haruki-cloud/utils/logger"

	"github.com/gofiber/fiber/v3"
)

func probeApp(output *bytes.Buffer, opts accessLogOptions, readyStatus *int) *fiber.App {
	app := fiber.New()
	app.Use(accessLogMiddlewareWithOptions(harukiLogger.NewLogger("HTTP", "INFO", output), opts))
	app.Get(readinessProbePath, func(c fiber.Ctx) error {
		return c.SendStatus(*readyStatus)
	})
	app.Get("/other", func(c fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func TestAccessLogSkipsSuccessfulProbesOnly(t *testing.T) {
	var output bytes.Buffer
	status := fiber.StatusOK
	app := probeApp(&output, accessLogOptions{}, &status)
	before := suppressedProbeLogs.Value()

	for range 3 {
		if _, err := app.Test(httptest.NewRequest("GET", readinessProbePath, nil)); err != nil {
			t.Fatal(err)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("successful probes should not be access-logged: %q", output.String())
	}
	if got := suppressedProbeLogs.Value() - before; got != 3 {
		t.Fatalf("suppressed counter advanced by %d, want 3", got)
	}

	status = fiber.StatusServiceUnavailable
	if _, err := app.Test(httptest.NewRequest("GET", readinessProbePath, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "http_route=/readyz") || !strings.Contains(output.String(), "status_code=503") {
		t.Fatalf("a failed probe must be logged: %q", output.String())
	}

	output.Reset()
	if _, err := app.Test(httptest.NewRequest("GET", "/other", nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "http_route=/other") {
		t.Fatalf("ordinary requests stay logged: %q", output.String())
	}
}

func TestAccessLogProbesCanBeEnabled(t *testing.T) {
	var output bytes.Buffer
	status := fiber.StatusOK
	app := probeApp(&output, accessLogOptions{logProbes: true}, &status)
	if _, err := app.Test(httptest.NewRequest("GET", readinessProbePath, nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "http_route=/readyz") {
		t.Fatalf("access_log_probes=true should log probes: %q", output.String())
	}
}

func TestCreateFiberAppSkipsProbeLogsByDefault(t *testing.T) {
	original := harukiConfig.Cfg
	t.Cleanup(func() { harukiConfig.Cfg = original })
	harukiConfig.Cfg.Backend.AccessLog = "structured"
	harukiConfig.Cfg.Backend.AccessLogPath = ""
	harukiConfig.Cfg.Backend.AccessLogProbes = false
	before := suppressedProbeLogs.Value()
	app := createFiberApp(harukiLogger.NewLogger("Main", "ERROR", &bytes.Buffer{}))
	if _, err := app.Test(httptest.NewRequest("GET", readinessProbePath, nil)); err != nil {
		t.Fatal(err)
	}
	if suppressedProbeLogs.Value() != before+1 {
		t.Fatal("createFiberApp should skip the successful probe")
	}
}
