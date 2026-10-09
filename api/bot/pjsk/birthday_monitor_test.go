package pjsk

import (
	"context"
	"io"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/subscription"

	"github.com/gofiber/fiber/v3"
	"haruki-cloud/internal/i18n"
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
	assertSingleTextMessageContains(t, body, i18n.Unavailable(i18n.M("subscription.birthday.feature")).String())
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

func birthdayMonitorTestReply(t *testing.T, text string) []byte {
	t.Helper()
	app := birthdayMonitorTestApp(&birthdayMonitorTestGuard{allow: true})
	resp, err := app.Test(newBotPOSTRequest(botPJSKPath(birthdayMonitorCommandPath), BotCommandRequest{
		Platform:        "qq",
		PlatformUserID:  "12345",
		PlatformGroupID: "67890",
		SelfID:          "self",
		MatchedCommand:  "/烤森生日监听",
		Message:         onebot11.Message{onebot11.Text(text)},
	}))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	return body
}

// The monitor route answers -help with its own help document.
func TestBirthdayMonitorHandlerServesHelp(t *testing.T) {
	body := birthdayMonitorTestReply(t, "/烤森生日监听 -help")
	assertSingleTextMessageContains(t, body, "烤森生日材料监听")
	assertSingleTextMessageContains(t, body, "/烤森生日取消监听")
}

// A usage error ends with the help pointer of the command as typed.
func TestBirthdayMonitorUsageErrorPointsToHelp(t *testing.T) {
	text := "/jp烤森生日监听 xyz"
	_, err := subscription.ParseBirthdayMonitorCommand(text)
	got := commandErrorReply(context.Background(), err, birthdayMonitorCommandPath, birthdayMonitorHelpCommand(text))
	if want := i18n.Usage("/jp烤森生日监听").String(); !strings.HasSuffix(got, want) {
		t.Fatalf("usage error reply = %q, want it to end with %q", got, want)
	}
}

func TestBirthdayMonitorHelpCommandKeepsTypedPrefix(t *testing.T) {
	for text, want := range map[string]string{
		"/烤森生日监听 xyz":                 "/烤森生日监听",
		"/jp烤森生日取消监听 u2":              "/jp烤森生日取消监听",
		"/mysekai birthday monitor x": "/mysekai birthday monitor",
		"/查曲 x":                       "",
	} {
		if got := birthdayMonitorHelpCommand(text); got != want {
			t.Errorf("birthdayMonitorHelpCommand(%q) = %q, want %q", text, got, want)
		}
	}
}
