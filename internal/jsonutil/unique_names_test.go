package jsonutil

import "testing"

func TestUnmarshalUniqueNamesChecksInsideRawMessages(t *testing.T) {
	for _, data := range []string{
		`{"root":1,"root":2}`,
		`{"root":{"nested":1,"nested":2}}`,
		`{"root":[{"nested":1,"nes\u0074ed":2}]}`,
	} {
		var document map[string]RawMessage
		if err := UnmarshalUniqueNames([]byte(data), &document); err == nil {
			t.Fatal("duplicate names must be rejected, including inside raw messages")
		}
		if err := Unmarshal([]byte(data), &document); err != nil {
			t.Fatalf("ordinary V1-compatible Unmarshal changed: %v", err)
		}
	}
	var document map[string]RawMessage
	if err := UnmarshalUniqueNames([]byte(`{"root":{"id":7057130168569158401},"other":{"id":2}}`), &document); err != nil {
		t.Fatal(err)
	}
	if string(document["root"]) != `{"id":7057130168569158401}` {
		t.Fatalf("raw field changed: %s", document["root"])
	}
}
