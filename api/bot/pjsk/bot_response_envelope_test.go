package pjsk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"haruki-cloud/api"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/onebot11"

	"github.com/gofiber/fiber/v3"
	"github.com/shamaton/msgpack/v3"
)

func encodeBotResponseEnvelope(envelope botResponseEnvelope) (encodedBotResponse, error) {
	return encodeBotResponseEnvelopeContext(context.Background(), envelope)
}

type botResponseEnvelopeWire[T any] struct {
	Status  int    `json:"status" msgpack:"status"`
	Message string `json:"message" msgpack:"message"`
	Data    T      `json:"data" msgpack:"data"`
}

type botResponseEnvelopeTestData struct {
	Items []string `json:"items" msgpack:"items"`
}

func TestEncodeBotResponseEnvelopePreservesLogicalShape(t *testing.T) {
	data := botResponseEnvelopeTestData{Items: []string{"first", "second"}}
	envelope := newBotResponseEnvelope(fiber.StatusAccepted, "queued", data, "ignored")
	if envelope.HTTPStatus != fiber.StatusAccepted || envelope.Message != "queued" {
		t.Fatalf("unexpected envelope metadata: %+v", envelope)
	}
	if got, ok := envelope.Data.(botResponseEnvelopeTestData); !ok || len(got.Items) != 2 {
		t.Fatalf("unexpected envelope data: %#v", envelope.Data)
	}

	encoded, err := encodeBotResponseEnvelope(envelope)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	data.Items[0] = "mutated after encode"

	var jsonKeys map[string]json.RawMessage
	if err := json.Unmarshal(encoded.JSONBody, &jsonKeys); err != nil {
		t.Fatalf("decode JSON keys: %v", err)
	}
	if len(jsonKeys) != 3 || jsonKeys["status"] == nil || jsonKeys["message"] == nil || jsonKeys["data"] == nil {
		t.Fatalf("JSON shape = %#v", jsonKeys)
	}
	var jsonWire botResponseEnvelopeWire[botResponseEnvelopeTestData]
	if err := json.Unmarshal(encoded.JSONBody, &jsonWire); err != nil {
		t.Fatalf("decode JSON envelope: %v", err)
	}
	assertBotResponseEnvelopeWire(t, jsonWire)

	var msgPackKeys map[string]any
	if err := msgpack.Unmarshal(encoded.MsgPackBody, &msgPackKeys); err != nil {
		t.Fatalf("decode MsgPack keys: %v", err)
	}
	if len(msgPackKeys) != 3 {
		t.Fatalf("MsgPack shape = %#v", msgPackKeys)
	}
	for _, key := range []string{"status", "message", "data"} {
		if _, ok := msgPackKeys[key]; !ok {
			t.Fatalf("MsgPack response missing %q: %#v", key, msgPackKeys)
		}
	}
	var msgPackWire botResponseEnvelopeWire[botResponseEnvelopeTestData]
	if err := msgpack.Unmarshal(encoded.MsgPackBody, &msgPackWire); err != nil {
		t.Fatalf("decode MsgPack envelope: %v", err)
	}
	assertBotResponseEnvelopeWire(t, msgPackWire)
}

func assertBotResponseEnvelopeWire(t *testing.T, wire botResponseEnvelopeWire[botResponseEnvelopeTestData]) {
	t.Helper()
	if wire.Status != fiber.StatusAccepted || wire.Message != "queued" {
		t.Fatalf("unexpected wire metadata: %+v", wire)
	}
	if len(wire.Data.Items) != 2 || wire.Data.Items[0] != "first" || wire.Data.Items[1] != "second" {
		t.Fatalf("unexpected wire data: %+v", wire.Data)
	}
}

func TestEncodeBotResponseEnvelopePreservesNullAndEmptyData(t *testing.T) {
	t.Run("omitted data is null", func(t *testing.T) {
		encoded, err := encodeBotResponseEnvelope(newBotResponseEnvelope(fiber.StatusOK, api.ResponseOK))
		if err != nil {
			t.Fatalf("encode envelope: %v", err)
		}
		assertEncodedDataShape(t, encoded, "null", true)
	})

	t.Run("empty message is an array", func(t *testing.T) {
		encoded, err := encodeBotResponseEnvelope(newBotResponseEnvelope(
			fiber.StatusOK,
			api.ResponseOK,
			make(onebot11.Message, 0),
		))
		if err != nil {
			t.Fatalf("encode envelope: %v", err)
		}
		assertEncodedDataShape(t, encoded, "[]", false)
	})
}

func assertEncodedDataShape(t *testing.T, encoded encodedBotResponse, wantJSON string, wantNil bool) {
	t.Helper()
	var jsonWire botResponseEnvelopeWire[json.RawMessage]
	if err := json.Unmarshal(encoded.JSONBody, &jsonWire); err != nil {
		t.Fatalf("decode JSON envelope: %v", err)
	}
	if string(jsonWire.Data) != wantJSON {
		t.Fatalf("JSON data = %s, want %s", jsonWire.Data, wantJSON)
	}

	var msgPackWire botResponseEnvelopeWire[any]
	if err := msgpack.Unmarshal(encoded.MsgPackBody, &msgPackWire); err != nil {
		t.Fatalf("decode MsgPack envelope: %v", err)
	}
	if wantNil {
		if msgPackWire.Data != nil {
			t.Fatalf("MsgPack data = %#v, want nil", msgPackWire.Data)
		}
		return
	}
	items, ok := msgPackWire.Data.([]any)
	if !ok || items == nil || len(items) != 0 {
		t.Fatalf("MsgPack data = %#v, want empty array", msgPackWire.Data)
	}
}

func TestWriteEncodedBotResponseSelectsCurrentRequestTransport(t *testing.T) {
	encoded, err := encodeBotResponseEnvelope(newBotResponseEnvelope(
		fiber.StatusCreated,
		"created",
		onebot11.Message{onebot11.Text("done")},
	))
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}

	tests := []struct {
		name     string
		secure   bool
		wantType string
		wantBody []byte
	}{
		{name: "JSON", wantType: api.ContentTypeJSON, wantBody: encoded.JSONBody},
		{name: "MsgPack plaintext", secure: true, wantType: api.ContentTypeMsgPack, wantBody: encoded.MsgPackBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/response", func(c fiber.Ctx) error {
				if tt.secure {
					c.Locals("secure_noise", true)
				}
				return writeEncodedBotResponse(c, encoded)
			})

			response, err := app.Test(httptest.NewRequest("GET", "/response", nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != fiber.StatusCreated {
				t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusCreated)
			}
			if got := response.Header.Get(fiber.HeaderContentType); got != tt.wantType {
				t.Fatalf("content type = %q, want %q", got, tt.wantType)
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if !bytes.Equal(body, tt.wantBody) {
				t.Fatalf("body = %x, want %x", body, tt.wantBody)
			}
		})
	}
}

type botResponseJSONOnly func()

func (botResponseJSONOnly) MarshalJSON() ([]byte, error) {
	return []byte(`"JSON-only value"`), nil
}

func TestEncodeBotResponseEnvelopeTraceMeasuresAttemptedFormats(t *testing.T) {
	tests := []struct {
		name    string
		data    any
		wantErr string
		wantOps map[string]int
	}{
		{name: "both formats", data: "data", wantOps: map[string]int{"response.json_encode": 1, "response.msgpack_encode": 1}},
		{name: "JSON error", data: make(chan int), wantErr: "as JSON", wantOps: map[string]int{"response.json_encode": 1}},
		{name: "MsgPack error", data: botResponseJSONOnly(nil), wantErr: "as MsgPack", wantOps: map[string]int{"response.json_encode": 1, "response.msgpack_encode": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(context.Background())
			_, err := encodeBotResponseEnvelopeContext(ctx, newBotResponseEnvelope(200, "ok", tt.data))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("encode: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			snapshot := trace.Snapshot()
			if len(snapshot.Phases) != 0 {
				t.Fatalf("encoding introduced phases: %+v", snapshot.Phases)
			}
			gotOps := make(map[string]int, len(snapshot.Operations))
			for _, op := range snapshot.Operations {
				gotOps[op.Name] = op.Count
			}
			if !reflect.DeepEqual(gotOps, tt.wantOps) {
				t.Fatalf("operations = %v, want %v", gotOps, tt.wantOps)
			}
		})
	}
}

func TestWriteEncodedBotResponseTraceDoesNotReencode(t *testing.T) {
	encoded, err := encodeBotResponseEnvelope(newBotResponseEnvelope(200, "ok", "data"))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, secure := range []bool{false, true} {
		t.Run(fmt.Sprintf("secure=%t", secure), func(t *testing.T) {
			ctx, trace := commandtrace.WithTrace(context.Background())
			app := fiber.New()
			app.Get("/response", func(c fiber.Ctx) error {
				c.SetContext(ctx)
				if secure {
					c.Locals("secure_noise", true)
				}
				return writeEncodedBotResponse(c, encoded)
			})
			response, err := app.Test(httptest.NewRequest("GET", "/response", nil))
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			_ = response.Body.Close()
			snapshot := trace.Snapshot()
			if len(snapshot.Phases) != 1 || snapshot.Phases[0].Name != "response_encode" || snapshot.Phases[0].Count != 1 {
				t.Fatalf("phases = %+v", snapshot.Phases)
			}
			if len(snapshot.Operations) != 1 || snapshot.Operations[0].Name != "response.body_set" || snapshot.Operations[0].Count != 1 {
				t.Fatalf("operations = %+v; writing preencoded data must not encode it again", snapshot.Operations)
			}
		})
	}
}
