package pjsk

import (
	"github.com/gofiber/fiber/v3"
	"haruki-cloud/internal/observability/commandtrace"
	"testing"
)

func TestPayloadSessionPhaseEndsBeforeDownstream(t *testing.T) {
	client, token := payloadSessionTestSetup(t)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		ctx, _ := commandtrace.WithTrace(c.Context())
		c.SetContext(ctx)
		return c.Next()
	})
	reached := false
	app.Post("/api/v2/bot/:botId/pjsk/test", verifyBotSessionFromPayload(client, nil, nil), func(c fiber.Ctx) error {
		reached = true
		snapshot := commandtrace.FromContext(c.Context()).Snapshot()
		for _, phase := range snapshot.Phases {
			if phase.Name == "session_auth" && phase.Count == 1 {
				return c.SendStatus(204)
			}
		}
		t.Error("session_auth must finish before downstream starts")
		return c.SendStatus(500)
	})
	status, _ := postJSON(t, app, "/api/v2/bot/42/pjsk/test", `{"session_token":"`+token+`"}`)
	if status != 204 || !reached {
		t.Fatalf("status=%d reached=%v", status, reached)
	}
}
