package server

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"haruki-cloud/internal/observability/commandtrace"
	harukiLogger "haruki-cloud/utils/logger"
)

func TestAccessLogAggregatesOperationsWithoutAddingPhases(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[failed], func(t *testing.T) {
			var output bytes.Buffer
			parentCtx, parentTrace := commandtrace.WithTrace(t.Context())
			app := fiber.New()
			app.Use(func(c fiber.Ctx) error { c.SetContext(parentCtx); return c.Next() })
			app.Use(accessLogMiddleware(harukiLogger.NewLogger("HTTP", "INFO", &output)))
			app.Get("/api/v2/public/chunithm/music/:id", func(c fiber.Ctx) error {
				if commandtrace.FromContext(c.Context()) != parentTrace {
					t.Error("access logger replaced existing trace")
				}
				commandtrace.RecordOperation(c.Context(), "db.query", time.Millisecond)
				commandtrace.RecordOperation(c.Context(), "api.cache_read", 2*time.Millisecond)
				if failed {
					return errors.New("private failure text")
				}
				return c.SendString("ok")
			})
			response, err := app.Test(httptest.NewRequest("GET", "/api/v2/public/chunithm/music/private-id?token=private-query", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			line := output.String()
			for _, want := range []string{"event=http_request", "http_route=/api/v2/public/chunithm/music/:id", "operation_stats_kind=inclusive", "operation_stats.db.query.count=1", "operation_stats.api.cache_read.count=1"} {
				if !strings.Contains(line, want) {
					t.Errorf("missing %q in %s", want, line)
				}
			}
			if strings.Count(line, "event=http_request") != 1 {
				t.Fatalf("expected one access summary: %s", line)
			}
			for _, secret := range []string{"private-id", "private-query", "private failure text"} {
				if strings.Contains(line, secret) {
					t.Fatalf("sensitive value in summary: %s", line)
				}
			}
			if len(parentTrace.Snapshot().Phases) != 0 {
				t.Fatal("access logger added an overlapping exclusive phase")
			}
			if failed && response.StatusCode != 500 {
				t.Fatalf("failure status = %d", response.StatusCode)
			}
		})
	}
}
