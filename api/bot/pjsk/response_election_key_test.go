package pjsk

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"

	"github.com/shamaton/msgpack/v3"
)

func TestResponseElectionIdentityIncludesRequestSemantics(t *testing.T) {
	base := responseElectionIdentityTestRequest()
	baseIdentity := responseElectionIdentity(base)
	tests := []struct {
		name   string
		mutate func(*BotCommandRequest)
	}{
		{name: "platform", mutate: func(req *BotCommandRequest) { req.Platform = "discord" }},
		{name: "group", mutate: func(req *BotCommandRequest) { req.PlatformGroupID = "group-2" }},
		{name: "user", mutate: func(req *BotCommandRequest) { req.PlatformUserID = "user-2" }},
		{name: "matched command", mutate: func(req *BotCommandRequest) { req.MatchedCommand = "/另一条指令" }},
		{name: "arguments", mutate: func(req *BotCommandRequest) { req.Message[0] = onebot11.Text("/查卡 1002") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := responseElectionIdentityTestRequest()
			test.mutate(&changed)
			if got := responseElectionIdentity(changed); got == baseIdentity {
				t.Fatalf("identity did not change after mutating %s", test.name)
			}
		})
	}
}

func TestResponseElectionIdentityExcludesBotSpecificSelfID(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.SelfID = "bot-self-a"
	second := responseElectionIdentityTestRequest()
	second.SelfID = "bot-self-b"

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("SelfID changed event identity: got %s, want %s", got, want)
	}
}

func TestResponseElectionIdentityExcludesNormalCommandNotifyPreference(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.NotifyEmpty = false
	second := responseElectionIdentityTestRequest()
	second.NotifyEmpty = true

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("unused notify preference changed event identity: got %s, want %s", got, want)
	}
}

func TestResponseElectionIdentityExcludesClientConfiguration(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	second := responseElectionIdentityTestRequest()
	second.Server = "cn"
	second.EnableParamEcho = !first.EnableParamEcho

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("client configuration changed event identity: got %s, want %s", got, want)
	}
}

func TestResponseElectionIdentityIgnoresNonTextAdapterSegments(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.SelfID = "bot-self-a"
	first.Message = onebot11.Message{onebot11.At(first.SelfID), onebot11.Text("/查卡 1001")}
	second := responseElectionIdentityTestRequest()
	second.SelfID = "bot-self-b"
	second.Message = onebot11.Message{onebot11.Image("adapter.png", ""), onebot11.Text("/查卡 1001"), onebot11.At("different-user")}

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("non-text adapter segments changed identity: got %s, want %s", got, want)
	}
	second.Message[1] = onebot11.Text("/查卡 1002")
	if got, same := responseElectionIdentity(second), responseElectionIdentity(first); got == same {
		t.Fatalf("changed command arguments did not change identity: %s", got)
	}
}

func TestResponseElectionIdentityNormalizesCommandWhitespaceAndCase(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.MatchedCommand = "/CARD"
	first.Message = onebot11.Message{onebot11.Text(" /CARD   +662 ")}
	second := responseElectionIdentityTestRequest()
	second.MatchedCommand = "/card"
	second.Message = onebot11.Message{onebot11.Text("/card "), onebot11.Text("+662")}

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("equivalent command arguments changed identity: got %s, want %s", got, want)
	}
}

func TestResponseElectionIdentityIsJSONMsgPackEquivalent(t *testing.T) {
	original := responseElectionIdentityTestRequest()
	original.Message = append(original.Message, onebot11.Segment{
		Type: "meta",
		Data: map[string]any{
			"count":  7,
			"nested": map[string]any{"enabled": true, "label": "same"},
		},
	})

	jsonPayload, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	var fromJSON BotCommandRequest
	if err := json.Unmarshal(jsonPayload, &fromJSON); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	msgpackPayload, err := msgpack.Marshal(original)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}
	var fromMsgPack BotCommandRequest
	if err := msgpack.Unmarshal(msgpackPayload, &fromMsgPack); err != nil {
		t.Fatalf("msgpack unmarshal: %v", err)
	}

	if got, want := responseElectionIdentity(fromMsgPack), responseElectionIdentity(fromJSON); got != want {
		t.Fatalf("transport changed identity:\nmsgpack=%s\njson=%s", got, want)
	}
}

func TestResponseElectionIdentityUsesLengthPrefixes(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.Platform = "a|b"
	first.PlatformGroupID = "c"
	second := responseElectionIdentityTestRequest()
	second.Platform = "a"
	second.PlatformGroupID = "b|c"

	if got, other := responseElectionIdentity(first), responseElectionIdentity(second); got == other {
		t.Fatalf("field-boundary collision produced identity %s", got)
	}
}

func TestResponseElectionIdentityAndRedisKeys(t *testing.T) {
	request := responseElectionIdentityTestRequest()
	identity := responseElectionIdentity(request)
	if len(identity) != sha256HexLength {
		t.Fatalf("identity length = %d, want %d", len(identity), sha256HexLength)
	}
	if _, err := hex.DecodeString(identity); err != nil {
		t.Fatalf("identity is not hex: %v", err)
	}

	stateKey := responseElectionStateKey(identity)
	candidatesKey := responseElectionCandidatesKey(identity)
	wantSlot := "{" + identity + "}"
	if !strings.Contains(stateKey, wantSlot) || !strings.Contains(candidatesKey, wantSlot) {
		t.Fatalf("keys do not share Redis hash tag: state=%q candidates=%q", stateKey, candidatesKey)
	}
	if want := "haruki:bot:response-election:" + wantSlot + ":state"; stateKey != want {
		t.Fatalf("state key = %q, want %q", stateKey, want)
	}
	if want := "haruki:bot:response-election:" + wantSlot + ":candidates"; candidatesKey != want {
		t.Fatalf("candidates key = %q, want %q", candidatesKey, want)
	}
	if want := "haruki:bot:dedup:" + identity; dedupKey(request) != want {
		t.Fatalf("dedup key = %q, want %q", dedupKey(request), want)
	}
}

const sha256HexLength = 64

func responseElectionIdentityTestRequest() BotCommandRequest {
	return BotCommandRequest{
		Platform:        "qq",
		PlatformUserID:  "user-1",
		PlatformGroupID: "group-1",
		SelfID:          "bot-self",
		Server:          "jp",
		NotifyEmpty:     true,
		MatchedCommand:  "/查卡",
		Message: onebot11.Message{
			onebot11.Text("/查卡 1001"),
			onebot11.At("123456"),
			onebot11.Image("image.png", "https://example.invalid/image.png"),
		},
		EnableParamEcho: true,
	}
}
