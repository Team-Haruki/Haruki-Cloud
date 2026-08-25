package jsonutil

import (
	"bytes"
	"testing"
)

func TestMarshalPreservesStableLegacyWireFormat(t *testing.T) {
	encoded, err := Marshal(map[string]any{
		"slice": []string(nil),
		"b":     2,
		"a":     1,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"a":1,"b":2,"slice":null}`; got != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}
}

func TestDecoderUseNumberPreservesIntegerPrecision(t *testing.T) {
	decoder := NewDecoder(bytes.NewBufferString(`{"value":9007199254740993}`))
	decoder.UseNumber()

	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	number, ok := decoded["value"].(Number)
	if !ok {
		t.Fatalf("decoded number type = %T, want json.Number", decoded["value"])
	}
	if got, want := number.String(), "9007199254740993"; got != want {
		t.Fatalf("decoded number = %s, want %s", got, want)
	}
}

func TestEncoderTerminatesValueWithNewline(t *testing.T) {
	var buffer bytes.Buffer
	if err := NewEncoder(&buffer).Encode(map[string]int{"value": 1}); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if got, want := buffer.String(), "{\"value\":1}\n"; got != want {
		t.Fatalf("Encode() = %q, want %q", got, want)
	}
}
