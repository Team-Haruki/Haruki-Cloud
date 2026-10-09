package onebot11

import (
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
)

func TestLocalizedTextRendersPerClient(t *testing.T) {
	const secret = "ECHO_SENTINEL"
	msg := i18n.M("alias.record.pending", i18n.Data{
		"ReviewID": 12, "Kind": i18n.M("alias.kind.music"), "Name": "Tell Your World", "EntityID": 74,
		"UserAlias": i18n.UserText(secret),
	})
	message := Message{At("10001"), LocalizedText(msg)}
	if !message.HasLocalizedText() || (Message{Text("x")}).HasLocalizedText() {
		t.Fatal("HasLocalizedText")
	}

	noEcho := message.Render(i18n.RenderOptions{NoEcho: true})
	echo := message.Render(i18n.RenderOptions{})
	if noEcho.HasLocalizedText() || echo.HasLocalizedText() {
		t.Fatal("rendered messages still hold localized segments")
	}
	if got := noEcho[1].Data.(TextData).Text; strings.Contains(got, secret) || got != msg.Render(i18n.RenderOptions{NoEcho: true}) {
		t.Fatalf("without echo = %q", got)
	}
	if got := echo[1].Data.(TextData).Text; got != msg.String() {
		t.Fatalf("with echo = %q", got)
	}
	if noEcho[0] != message[0] {
		t.Fatalf("other segments changed: %+v", noEcho[0])
	}

	// A segment encoded without Render never carries the user input.
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "待审核别名 #12") {
		t.Fatalf("unrendered encoding = %s", raw)
	}
}
