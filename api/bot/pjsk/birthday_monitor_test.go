package pjsk

import (
	"context"
	"io"
	"testing"

	"haruki-cloud/internal/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/subscription"

	"github.com/gofiber/fiber/v3"
)

func TestBirthdayMonitorCommandTextPrependsMatchedCommandForArgumentOnlyMessage(t *testing.T) {
	req := BotCommandRequest{
		MatchedCommand: "/烤森生日监听",
		Message:        onebot11.Message{onebot11.Text("u2 钻石")},
	}
	text := birthdayMonitorCommandText(req)
	cmd, err := subscription.ParseBirthdayMonitorCommand(text)
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand(%q) returned error: %v", text, err)
	}
	if cmd.Selector != "u2" {
		t.Fatalf("selector = %q, want u2", cmd.Selector)
	}
}

func TestBirthdayMonitorCommandTextSupportsRegionPrefixedMatchedCommand(t *testing.T) {
	req := BotCommandRequest{
		MatchedCommand: "/jp烤森生日监听",
		Message:        onebot11.Message{onebot11.Text("钻石 10")},
	}
	text := birthdayMonitorCommandText(req)
	cmd, err := subscription.ParseBirthdayMonitorCommand(text)
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand(%q) returned error: %v", text, err)
	}
	if !cmd.RegionExplicit || cmd.Region != "jp" {
		t.Fatalf("region = %q explicit=%t, want jp explicit", cmd.Region, cmd.RegionExplicit)
	}
	if cmd.DurationMinutes != 10 {
		t.Fatalf("duration = %d, want 10", cmd.DurationMinutes)
	}
}

func TestBirthdayMonitorHandlerDropsWhenGuardRejects(t *testing.T) {
	guard := &birthdayMonitorTestGuard{allow: false}
	app := birthdayMonitorTestApp(guard)

	resp, err := app.Test(newBotPOSTRequest(botPJSKPath(birthdayMonitorCommandPath), BotCommandRequest{
		Platform:        "qq",
		PlatformUserID:  "12345",
		PlatformGroupID: "67890",
		SelfID:          "self",
		Server:          "jp",
		MatchedCommand:  "/烤森生日监听",
		Message:         onebot11.Message{onebot11.Text("/烤森生日监听 钻石")},
	}))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	message := decodeSuccessMessage(t, body)
	if len(message) != 0 {
		t.Fatalf("expected empty message for dedup drop, got %+v", message)
	}
	if guard.acquired != 1 {
		t.Fatalf("guard acquired = %d, want 1", guard.acquired)
	}
	if guard.completed != 0 {
		t.Fatalf("guard completed = %d, want 0", guard.completed)
	}
	if guard.request.Platform != "qq" || guard.request.PlatformGroupID != "67890" || guard.request.PlatformUserID != "12345" || guard.request.MatchedCommand != "/烤森生日监听" {
		t.Fatalf("unexpected guarded request: %+v", guard.request)
	}
}

func TestBirthdayMonitorHandlerMarksGuardCompleteAfterVisibleResponse(t *testing.T) {
	guard := &birthdayMonitorTestGuard{allow: true}
	app := birthdayMonitorTestApp(guard)

	resp, err := app.Test(newBotPOSTRequest(botPJSKPath(birthdayMonitorCommandPath), BotCommandRequest{
		Platform:        "qq",
		PlatformUserID:  "12345",
		PlatformGroupID: "67890",
		SelfID:          "self",
		Server:          "jp",
		MatchedCommand:  "/烤森生日监听",
		Message:         onebot11.Message{onebot11.Text("/烤森生日监听 钻石")},
	}))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	assertSingleTextMessageContains(t, body, "生日材料监听服务未就绪")
	if guard.acquired != 1 {
		t.Fatalf("guard acquired = %d, want 1", guard.acquired)
	}
	if guard.completed != 1 {
		t.Fatalf("guard completed = %d, want 1", guard.completed)
	}
}

func birthdayMonitorTestApp(guard commandRequestGuard) *fiber.App {
	app := fiber.New()
	bot := app.Group(botRouteBase + "/:botId")
	pjsk := bot.Group("/pjsk")
	pjsk.Post("/"+birthdayMonitorCommandPath, makeBirthdayMonitorHandler(&renderapp.App{}, guard))
	return app
}

type birthdayMonitorTestGuard struct {
	allow     bool
	acquired  int
	completed int
	request   BotCommandRequest
}

func (g *birthdayMonitorTestGuard) Acquire(_ context.Context, req BotCommandRequest) requestGuardLease {
	g.acquired++
	g.request = req
	return requestGuardLease{proceed: g.allow, token: "test-owner"}
}

func (g *birthdayMonitorTestGuard) MarkComplete(_ context.Context, _ BotCommandRequest, _ requestGuardLease) {
	g.completed++
}
