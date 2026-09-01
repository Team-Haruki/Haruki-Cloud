package config

import "testing"

func TestEnvStringMapAdditionalFormats(t *testing.T) {
	const name = "HARUKI_TEST_STRING_MAP"
	t.Setenv(name, "jp=https://jp.example; en = https://en.example")
	values := map[string]string{}
	if err := envStringMap(name, &values); err != nil || values["jp"] == "" || values["en"] == "" {
		t.Fatalf("CSV map = %#v, %v", values, err)
	}
	t.Setenv(name, "missing-value")
	if err := envStringMap(name, &values); err == nil {
		t.Fatal("invalid CSV map was accepted")
	}
	t.Setenv(name, `{"":"https://example.com"}`)
	if err := envStringMap(name, &values); err == nil {
		t.Fatal("blank JSON map key was accepted")
	}
}

func TestEnvStringSliceRejectsInvalidJSON(t *testing.T) {
	const name = "HARUKI_TEST_STRING_SLICE"
	t.Setenv(name, "[")
	var values []string
	if err := envStringSlice(name, &values); err == nil {
		t.Fatal("invalid JSON slice was accepted")
	}
}
