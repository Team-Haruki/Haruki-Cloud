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
		{name: "server", mutate: func(req *BotCommandRequest) { req.Server = "cn" }},
		{name: "notify empty", mutate: func(req *BotCommandRequest) { req.NotifyEmpty = !req.NotifyEmpty }},
		{name: "parameter echo", mutate: func(req *BotCommandRequest) { req.EnableParamEcho = !req.EnableParamEcho }},
		{name: "message type", mutate: func(req *BotCommandRequest) { req.Message[0].Type = onebot11.TypeImage }},
		{name: "message data", mutate: func(req *BotCommandRequest) { req.Message[1] = onebot11.At("987654") }},
		{name: "message order", mutate: func(req *BotCommandRequest) { req.Message[0], req.Message[1] = req.Message[1], req.Message[0] }},
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

func TestResponseElectionIdentityNormalizesKnownServer(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.Server = "jp"
	second := responseElectionIdentityTestRequest()
	second.Server = " JP "

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("equivalent server changed event identity: got %s, want %s", got, want)
	}
}

func TestResponseElectionIdentityCanonicalizesSelfMentions(t *testing.T) {
	first := responseElectionIdentityTestRequest()
	first.SelfID = "bot-self-a"
	first.Message = onebot11.Message{onebot11.At(first.SelfID), onebot11.Text("/查卡 1001")}
	second := responseElectionIdentityTestRequest()
	second.SelfID = "bot-self-b"
	second.Message = onebot11.Message{onebot11.At(second.SelfID), onebot11.Text("/查卡 1001")}

	if got, want := responseElectionIdentity(second), responseElectionIdentity(first); got != want {
		t.Fatalf("bot-specific self mention changed identity: got %s, want %s", got, want)
	}
	second.Message[0] = onebot11.At("different-user")
	if got, same := responseElectionIdentity(second), responseElectionIdentity(first); got == same {
		t.Fatalf("non-self mention did not change identity: %s", got)
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
	identity := responseElectionIdentity(responseElectionIdentityTestRequest())
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
