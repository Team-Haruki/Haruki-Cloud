package drawing

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAssetCandidatesDropsBlanksAndDuplicates(t *testing.T) {
	got := AssetCandidates(" a.png ", "", "b.png", "a.png", "  ", "c.png")
	if want := (AssetKey{"a.png", "b.png", "c.png"}); !slices.Equal(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	if got.First() != "a.png" || got.Last() != "c.png" {
		t.Fatalf("first/last = %q/%q", got.First(), got.Last())
	}
	if AssetCandidates("", " ") != nil || AssetPath(" ") != nil {
		t.Fatal("blank candidates must give a nil key")
	}
	var empty AssetKey
	if empty.First() != "" || empty.Last() != "" {
		t.Fatal("empty key must have no first/last")
	}
	if got := AssetPath("x.png"); !slices.Equal(got, AssetKey{"x.png"}) {
		t.Fatalf("single path = %v", got)
	}
}

func TestAssetKeyMarshalKeepsSinglePathShape(t *testing.T) {
	type payload struct {
		Required AssetKey `json:"required"`
		Optional AssetKey `json:"optional,omitempty"`
	}
	for _, tc := range []struct {
		name string
		in   payload
		want string
	}{
		{"empty", payload{}, `{"required":""}`},
		{"single", payload{Required: AssetPath("a.png"), Optional: AssetPath("b.png")}, `{"required":"a.png","optional":"b.png"}`},
		{"list", payload{Required: AssetCandidates("a.png", "b.png")}, `{"required":["a.png","b.png"]}`},
	} {
		body, err := json.Marshal(tc.in)
		if err != nil || string(body) != tc.want {
			t.Fatalf("%s: marshal = %s (%v), want %s", tc.name, body, err, tc.want)
		}
	}
	// A single candidate marshals byte-identically to the plain string field
	// it replaced, so the render-cache key of such a payload is unchanged (C2).
	before, _ := json.Marshal(struct {
		IconPath string `json:"icon_path"`
	}{"asset/jp-assets/startapp/x.png"})
	after, _ := json.Marshal(InventoryItem{IconPath: AssetPath("asset/jp-assets/startapp/x.png")})
	var decoded map[string]any
	if err := json.Unmarshal(after, &decoded); err != nil || decoded["icon_path"] != "asset/jp-assets/startapp/x.png" || string(before) != `{"icon_path":"asset/jp-assets/startapp/x.png"}` {
		t.Fatalf("single-path wire shape changed: %s", after)
	}
}

func TestAssetKeyUnmarshal(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want AssetKey
	}{
		{`"a.png"`, AssetKey{"a.png"}},
		{`""`, nil},
		{`null`, nil},
		{`["a.png","b.png"]`, AssetKey{"a.png", "b.png"}},
		{` [] `, AssetKey{}},
	} {
		var key AssetKey
		if err := json.Unmarshal([]byte(tc.in), &key); err != nil || !slices.Equal(key, tc.want) {
			t.Fatalf("unmarshal %s = %v (%v), want %v", tc.in, key, err, tc.want)
		}
	}
	for _, bad := range []string{`[1]`, `1`, `{}`} {
		var key AssetKey
		if err := key.UnmarshalJSON([]byte(bad)); err == nil {
			t.Fatalf("unmarshal %s must fail", bad)
		}
	}
}
