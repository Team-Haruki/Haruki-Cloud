package mysekai

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

var snapshotTimeMemberDocs = []string{
	`{"upload_time":1700000000,"updatedResources":{"userMysekaiPhotos":[]}}`,
	`{"now":1700000000123,"upload_time":1700000000}`,
	`{"updatedResources":{"now":1700000500000,"upload_time":1700000400},"upload_time":1700000000,"now":1}`,
	`{"updatedResources":[1,2],"upload_time":5}`,
	`{"updatedResources":null,"now":"1700000000000"}`,
	`{"updatedResources":{"updatedResources":{"now":99}},"now":1.5e12}`,
	`{"now":12345678901234567890,"updatedResources":{"now":"x"}}`,
	`{"now":null,"upload_time":true}`,
	`{}`,
	`{"upload_time":1700000000} trailing`,
	`{"now":1e999999,"upload_time":-3}`,
	`{"now":1,"now":2,"updatedResources":{"now":3},"updatedResources":{"x":1}}`,
}

func TestSnapshotTimeMembersMatchFullDecode(t *testing.T) {
	for _, doc := range snapshotTimeMemberDocs {
		for _, rawMode := range []bool{false, true} {
			t.Run(fmt.Sprintf("raw=%t/%s", rawMode, doc), func(t *testing.T) {
				if !compareSnapshotTimeMembers(t, []byte(doc), rawMode) {
					t.Fatal("fast decode declined a well-formed object")
				}
			})
		}
	}
}

// compareSnapshotTimeMembers checks the streamed members against the full
// decode. It reports false when the fast path declined the document.
func compareSnapshotTimeMembers(t *testing.T, doc []byte, rawMode bool) bool {
	t.Helper()
	controller := &Controller{}
	if rawMode {
		controller.rawMySekaiJSON = doc
	}
	fast, ok := controller.decodeSnapshotTimeMembers(doc)
	full, err := controller.decodeSnapshotBytes(doc)
	if !ok {
		return false
	}
	if err != nil {
		t.Fatalf("fast decode accepted %q, full decode failed: %v", doc, err)
	}
	for _, key := range []string{"now", "upload_time"} {
		if !reflect.DeepEqual(fast[key], full[key]) {
			t.Fatalf("%s: fast %#v, full %#v", key, fast[key], full[key])
		}
	}
	fullUpdated, fullIsMap := full["updatedResources"].(map[string]any)
	fastUpdated, fastIsMap := fast["updatedResources"].(map[string]any)
	if fullIsMap != fastIsMap || (fullIsMap && !reflect.DeepEqual(fastUpdated["now"], fullUpdated["now"])) {
		t.Fatalf("updatedResources: fast %#v, full %#v", fast["updatedResources"], full["updatedResources"])
	}
	if got, want := resolveMysekaiSnapshotTimeMs(fast), resolveMysekaiSnapshotTimeMs(full); got != want {
		t.Fatalf("snapshot time: fast %d, full %d", got, want)
	}
	return true
}

func FuzzSnapshotTimeMembers(f *testing.F) {
	for _, doc := range snapshotTimeMemberDocs {
		f.Add([]byte(doc), true)
		f.Add([]byte(doc), false)
	}
	f.Fuzz(func(t *testing.T, doc []byte, rawMode bool) {
		compareSnapshotTimeMembers(t, doc, rawMode)
	})
}

func TestSnapshotStatusFallsBackToFullDecodeErrors(t *testing.T) {
	for _, doc := range []string{`{`, `[1]`, `"text"`} {
		controller := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData([]byte(doc))
		if _, ok := controller.decodeSnapshotTimeMembers([]byte(doc)); ok {
			t.Fatalf("fast decode accepted %q", doc)
		}
		_, fullErr := controller.decodeSnapshotBytes([]byte(doc))
		_, err := controller.SnapshotStatus("jp", time.Now())
		if err == nil || fullErr == nil || err.Error() != fullErr.Error() {
			t.Fatalf("SnapshotStatus(%q) error = %v, full decode error = %v", doc, err, fullErr)
		}
	}
}

func BenchmarkSnapshotStatus(b *testing.B) {
	var doc bytes.Buffer
	doc.WriteString(`{"updatedResources":{"userMysekaiHarvestMaps":[`)
	for i := 0; doc.Len() < 13<<20; i++ {
		if i > 0 {
			doc.WriteByte(',')
		}
		fmt.Fprintf(&doc, `{"mysekaiSiteId":%d,"userMysekaiSiteHarvestFixtures":[{"mysekaiSiteHarvestFixtureId":%d,"userMysekaiSiteHarvestFixtureStatus":"spawned","positionX":%d,"positionZ":%d}]}`, i%8, i, i%200, i%150)
	}
	doc.WriteString(`],"now":1700000000000},"upload_time":1700000000}`)
	controller := (&Controller{defaultRegion: renderregion.JP}).WithMySekaiData(doc.Bytes())
	b.Run("full_decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			merged, err := controller.decodeSnapshotBytes(doc.Bytes())
			if err != nil || resolveMysekaiSnapshotTimeMs(merged) == 0 {
				b.Fatal(err)
			}
		}
	})
	b.Run("status", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			status, err := controller.SnapshotStatus("jp", time.Now())
			if err != nil || status.LastUpdatedAt.IsZero() {
				b.Fatal(err)
			}
		}
	})
}
