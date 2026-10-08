package snapshot

import (
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func TestPayloadUploadTimeMatchesFullDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
	}{
		{"only member", `{"upload_time":5}`},
		{"leading member", `{"upload_time":5,"userCards":[{"id":1}]}`},
		{"after nested values", `{"a":[1,2,{"x":"}]"}],"b":{"c":null,"d":true},"upload_time":5,"z":"tail"}`},
		{"whitespace", " {\n \"a\" : \"x\" ,\r\n\t\"upload_time\" :\t-17 , \"b\": 1 } \n"},
		{"escaped strings", `{"a":"quote \" brace } \\","b":["\\\\"],"upload_time":5,"c":2}`},
		{"trailing member", `{"updatedResources":{"x":[{"y":"upload_time"}]},"upload_time":1710000000}`},
		{"trailing with whitespace", `{"a":1 , "upload_time" : 8 }  `},
		{"trailing duplicate wins", `{"upload_time":1,"x":0,"upload_time":2}`},
		{"string value inside other member", `{"a":"\"upload_time\":9","upload_time":5,"b":1}`},
		{"nested only", `{"a":{"upload_time":9},"b":2}`},
		{"case-insensitive name", `{"Upload_Time":7,"b":1}`},
		{"escaped name", `{"upload_time":5,"b":1}`},
		{"float", `{"upload_time":1.5,"b":1}`},
		{"exponent", `{"upload_time":1e3,"b":1}`},
		{"string", `{"upload_time":"5","b":1}`},
		{"null", `{"upload_time":null,"b":1}`},
		{"leading zero", `{"upload_time":05,"b":1}`},
		{"overflow", `{"upload_time":99999999999999999999,"b":1}`},
		{"array root", `[{"upload_time":5}]`},
		{"empty object", `{}`},
		{"zero", `{"upload_time":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, wantErr := parseTopLevelUploadTime([]byte(tc.doc))
			got, gotErr := payloadUploadTime([]byte(tc.doc))
			if got != want || (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("payloadUploadTime() = %d, %v; full decode = %d, %v", got, gotErr, want, wantErr)
			}
		})
	}
}

func TestScanTopLevelUploadTimeRejectsUnclearShapes(t *testing.T) {
	for _, doc := range []string{
		``, `{`, `{"a"`, `{"a":`, `{"a":"unterminated`, `{"a":[1,2`, `{"a":1 "b":2}`, `{"upload_time":5`, `{"upload_time":-}`,
		`{"a":}`, `{"Upload_time":1,"upload_time":2}`, `{"a\"b":1,"upload_time":2}`,
	} {
		if value, ok := scanTopLevelUploadTime([]byte(doc)); ok {
			t.Fatalf("scanTopLevelUploadTime(%q) = %d, want no answer", doc, value)
		}
	}
	for _, doc := range []string{`{"a":1}`, `x}`, `{"upload_time":5 }x`, `{"a":"upload_time":5}`, `{"x" "upload_time":5}`} {
		if value, ok := trailingUploadTime([]byte(doc)); ok {
			t.Fatalf("trailingUploadTime(%q) = %d, want no answer", doc, value)
		}
	}
}

func TestPayloadUploadTimeOnSyntheticPayloads(t *testing.T) {
	for _, data := range [][]byte{syntheticSuitePayload(256<<10, 1700000123), syntheticMySekaiPayload(256<<10, 1700000123)} {
		if got, err := payloadUploadTime(data); err != nil || got != 1700000123 {
			t.Fatalf("payloadUploadTime() = %d, %v", got, err)
		}
	}
}

// FuzzPayloadUploadTime checks the scanner against the full decode on every
// well-formed document without duplicate member names (Toolbox never sends
// duplicates; with them only the trailing fast path is exact).
func FuzzPayloadUploadTime(f *testing.F) {
	for _, seed := range []string{
		`{"upload_time":5}`, `{"a":[1,{"b":"}"}],"upload_time":12,"c":null}`, `{"x":{"upload_time":3}}`,
		`{"UPLOAD_TIME":4}`, `{"a":"\\\"","upload_time":-1}`, `{"upload_time":1.0}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var unique map[string]json.RawMessage
		if !json.Valid(data) || json.UnmarshalUniqueNames(data, &unique) != nil {
			return
		}
		want, wantErr := parseTopLevelUploadTime(data)
		got, gotErr := payloadUploadTime(data)
		if got != want || (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("payloadUploadTime(%q) = %d, %v; full decode = %d, %v", data, got, gotErr, want, wantErr)
		}
	})
}
