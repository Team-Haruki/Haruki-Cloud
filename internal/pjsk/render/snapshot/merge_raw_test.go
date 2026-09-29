package snapshot

import (
	"bytes"
	"reflect"
	"runtime"
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func assertMySekaiMergeEquivalent(t testing.TB, suite, mysekai []byte) {
	t.Helper()
	suiteBefore, mysekaiBefore := bytes.Clone(suite), bytes.Clone(mysekai)
	want, wantErr := mergeMySekaiDataDecoded(suite, mysekai)
	got, gotErr := mergeMySekaiData(suite, mysekai)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("merge error changed: decoded=%v raw=%v", wantErr, gotErr)
	}
	if !bytes.Equal(suite, suiteBefore) || !bytes.Equal(mysekai, mysekaiBefore) {
		t.Fatal("merge mutated an input snapshot")
	}
	if wantErr != nil {
		return
	}
	wantDocument, err := normalizeSnapshotDocument(want)
	if err != nil {
		t.Fatal(err)
	}
	gotDocument, err := normalizeSnapshotDocument(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDocument, wantDocument) {
		t.Fatal("raw-field merge changed decoded snapshot semantics")
	}
}

func TestRawMySekaiMergeMatchesDecodedRules(t *testing.T) {
	values := []string{`null`, `[]`, `[ ]`, `[1]`, `{}`, `{ "nested": [1, 2] }`, `""`, `" \t\u2003"`, `"value"`, `0`, `false`, `7057130168569158401`}
	keys := []string{"userMysekaiShops", "mysekaiUnknown", "userMysekaiCharacterTalks", "userGamedata", "unknown", "now", "upload_time", "source", "local_source", " userMysekaiUnknown "}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			for _, oldValue := range values {
				for _, newValue := range values {
					encodedKey, err := json.Marshal(key)
					if err != nil {
						t.Fatal(err)
					}
					suite := []byte(`{"unknownSuite":{"keep":7057130168569158401},` + string(encodedKey) + `:` + oldValue + `}`)
					for _, delta := range []string{
						`{` + string(encodedKey) + `:` + newValue + `}`,
						`{"updatedResources":{` + string(encodedKey) + `:` + newValue + `}}`,
						`{"updatedResources":{` + string(encodedKey) + `:` + newValue + `},` + string(encodedKey) + `:{"stale":true}}`,
					} {
						assertMySekaiMergeEquivalent(t, suite, []byte(delta))
						assertMySekaiMergeEquivalent(t, []byte(`{}`), []byte(delta))
					}
				}
			}
		})
	}
}

func TestRawMySekaiMergePreservesUnusualDocuments(t *testing.T) {
	for _, tc := range []struct {
		name, suite, mysekai string
	}{
		{"missing delta", `{"userCards":[]}`, `{}`},
		{"non-object updates", `{"mysekaiX":[1]}`, `{"updatedResources":[1],"mysekaiX":[2]}`},
		{"null updates", `{}`, `{"updatedResources":null,"mysekaiX":1}`},
		{"array export", `[{"userCards":[]}]`, `[{"updatedResources":{"mysekaiX":1}}]`},
		{"extended wrappers", `{"userGamedata":{"userId":{"$numberLong":"7057130168569158401"}},"unknown":{"$oid":"abcdef"}}`, `{"updatedResources":{"now":{"$date":{"$numberLong":"1781251200000"}},"mysekaiX":{"$numberDecimal":"1.5"}}}`},
		{"invalid ignored wrapper", `{}`, `{"unknown":{"$numberInt":false}}`},
		{"wrapper marker in text", `{"unknown":"$number is text"}`, `{"updatedResources":{"mysekaiX":1}}`},
		{"duplicate root", `{"unknown":1,"unknown":2}`, `{"mysekaiX":1}`},
		{"duplicate nested", `{"unknown":{"a":{"b":1},"a":{"c":2}}}`, `{"updatedResources":{"mysekaiX":{"a":1,"a":2}}}`},
		{"duplicate escaped name", `{"userGamedata":{"userId":"bad","user\u0049d":42}}`, `{}`},
		{"extra document", `{"unknown":1} {"ignored":2}`, `{"mysekaiX":1}`},
		{"extra invalid suffix", `{"unknown":1} trailing`, `{"mysekaiX":1}`},
		{"empty suite", ``, `{}`},
		{"null suite", `null`, `{}`},
		{"scalar suite", `7`, `{}`},
		{"invalid suite", `{"unknown":`, `{}`},
		{"null mysekai", `{}`, `null`},
		{"invalid mysekai", `{}`, `{"updatedResources":`},
		{"multi-document export", `[{},{}]`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertMySekaiMergeEquivalent(t, []byte(tc.suite), []byte(tc.mysekai))
		})
	}
}

func TestRawMySekaiMergeCollapsesDuplicateFieldsBeforeTypedDecode(t *testing.T) {
	suite := []byte(`{"userGamedata":{"userId":"bad","userId":7057130168569158401}}`)
	merged, err := mergeMySekaiData(suite, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var got RawUserData
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("duplicate fields were passed to the typed decoder: %v", err)
	}
	if got.UserGamedata.UserID != 7057130168569158401 {
		t.Fatalf("user ID changed: %d", got.UserGamedata.UserID)
	}
}

func TestRawMySekaiMergeDoesNotAliasInputOrOutput(t *testing.T) {
	suite := []byte(`{"unknown":{"keep":42}}`)
	delta := []byte(`{"updatedResources":{"mysekaiUnknown":[1,2,3]}}`)
	merged, err := mergeMySekaiData(suite, delta)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(merged)
	clear(suite)
	clear(delta)
	if !bytes.Equal(merged, want) {
		t.Fatal("merged snapshot aliases an input")
	}
}

func BenchmarkMergeMySekaiData(b *testing.B) {
	costumes := bytes.Repeat([]byte(`{"costumeId":1234,"obtainedAt":1781251200000,"isNew":false},`), 100000)
	materials := bytes.Repeat([]byte(`{"mysekaiMaterialId":1234,"quantity":999},`), 3000)
	suite := append([]byte(`{"userGamedata":{"userId":7057130168569158401},"userCostumes":[`), costumes[:len(costumes)-1]...)
	suite = append(suite, `]}`...)
	mysekai := append([]byte(`{"updatedResources":{"userMysekaiMaterials":[`), materials[:len(materials)-1]...)
	mysekai = append(mysekai, `]}}`...)
	assertMySekaiMergeEquivalent(b, suite, mysekai)
	for _, tc := range []struct {
		name  string
		merge func([]byte, []byte) ([]byte, error)
	}{
		{"Decoded", mergeMySekaiDataDecoded},
		{"RawFields", mergeMySekaiData},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(suite) + len(mysekai)))
			for b.Loop() {
				merged, err := tc.merge(suite, mysekai)
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(merged)
			}
		})
	}
}

func FuzzMergeMySekaiDataMatchesDecoded(f *testing.F) {
	f.Add([]byte(`{"userCards":[],"userMysekaiShops":[1],"unknown":{"id":7057130168569158401}}`), []byte(`{"updatedResources":{"userMysekaiShops":[]},"userMysekaiShops":[2]}`))
	f.Add([]byte(`[{"userGamedata":{"userId":{"$numberLong":"7057130168569158401"}}}]`), []byte(`{"updatedResources":{"now":{"$date":1}}}`))
	f.Add([]byte(`{"unknown":{"a":"wrong","a":42}}`), []byte(`{"mysekaiUnknown":null}`))
	f.Fuzz(func(t *testing.T, suite, mysekai []byte) {
		if len(suite)+len(mysekai) > 64<<10 {
			t.Skip()
		}
		assertMySekaiMergeEquivalent(t, suite, mysekai)
	})
}
