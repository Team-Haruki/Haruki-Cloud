package snapshot

import (
	"bytes"
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func TestNormalizeSnapshotJSONFastPathReturnsInputUnchanged(t *testing.T) {
	input := []byte(`{"userGamedata":{"userId":339871638031728641,"name":"player"},"upload_time":1781251200000}`)
	got, err := normalizeSnapshotJSON(input)
	if err != nil {
		t.Fatalf("normalizeSnapshotJSON: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("plain payload should be returned unchanged, got %s", got)
	}
}

func TestNormalizeSnapshotJSONStillRewritesExtendedJSON(t *testing.T) {
	input := []byte(`{"userGamedata":{"userId":{"$numberLong":"339871638031728641"}},"created":{"$date":{"$numberLong":"1781251200000"}}}`)
	got, err := normalizeSnapshotJSON(input)
	if err != nil {
		t.Fatalf("normalizeSnapshotJSON: %v", err)
	}
	var decoded struct {
		UserGamedata struct {
			UserID int64 `json:"userId"`
		} `json:"userGamedata"`
		Created int64 `json:"created"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if decoded.UserGamedata.UserID != 339871638031728641 {
		t.Fatalf("userId = %d, want 339871638031728641", decoded.UserGamedata.UserID)
	}
	if decoded.Created != 1781251200000 {
		t.Fatalf("created = %d, want 1781251200000", decoded.Created)
	}
}

func TestNormalizeSnapshotJSONStillUnwrapsTopLevelArray(t *testing.T) {
	input := []byte(`[{"userGamedata":{"userId":42}}]`)
	got, err := normalizeSnapshotJSON(input)
	if err != nil {
		t.Fatalf("normalizeSnapshotJSON: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("decode unwrapped payload: %v", err)
	}
	if _, ok := decoded["userGamedata"]; !ok {
		t.Fatalf("expected unwrapped document, got %s", got)
	}

	if _, err := normalizeSnapshotJSON([]byte(`[{"a":1},{"b":2}]`)); err == nil {
		t.Fatal("multi-document array must still be rejected")
	}
}

func TestNormalizeSnapshotDocumentDecodesAndNormalizes(t *testing.T) {
	document, err := normalizeSnapshotDocument([]byte(`[{"userId":{"$numberLong":"7"},"name":"x"}]`))
	if err != nil {
		t.Fatalf("normalizeSnapshotDocument: %v", err)
	}
	if got := document["name"]; got != "x" {
		t.Fatalf("name = %v, want x", got)
	}
	number, err := json.Marshal(document["userId"])
	if err != nil {
		t.Fatalf("marshal userId: %v", err)
	}
	if string(number) != "7" {
		t.Fatalf("userId = %s, want 7", number)
	}

	if _, err := normalizeSnapshotDocument([]byte(`"not an object"`)); err == nil {
		t.Fatal("non-object document must be rejected")
	}
	if _, err := normalizeSnapshotDocument(nil); err == nil {
		t.Fatal("empty document must be rejected")
	}
}

// Large integers above float64 precision must survive the mysekai merge
// byte-exact: the merge reuses the json.Number-typed normalization decode
// instead of re-decoding through float64.
func TestMergeMySekaiDataPreservesLargeIntegersExactly(t *testing.T) {
	suiteJSON := []byte(`{"userGamedata":{"userId":339871638031728641,"name":"player"},"upload_time":1781251200000}`)
	mysekaiJSON := []byte(`{"upload_time":1781254800000,"updatedResources":{"userMysekaiGates":[{"gateId":1}]}}`)

	merged, err := mergeMySekaiData(suiteJSON, mysekaiJSON)
	if err != nil {
		t.Fatalf("mergeMySekaiData: %v", err)
	}
	if !bytes.Contains(merged, []byte("339871638031728641")) {
		t.Fatalf("large userId must survive the merge exactly, got %s", merged)
	}
	if !bytes.Contains(merged, []byte(`"upload_time":1781254800000`)) {
		t.Fatalf("mysekai upload_time should win the merge, got %s", merged)
	}
}
