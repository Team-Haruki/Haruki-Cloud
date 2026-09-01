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

func TestEnvNoiseKeysFormats(t *testing.T) {
	t.Setenv("HARUKI_TEST_NOISE_KEYS", " k1 = aa ; k2=bb \n k3=cc ")
	var keys []NoiseStaticKeyConfig
	if err := envNoiseKeys("HARUKI_TEST_NOISE_KEYS", &keys); err != nil {
		t.Fatalf("envNoiseKeys: %v", err)
	}
	want := []NoiseStaticKeyConfig{{KeyID: "k1", PrivateKey: "aa"}, {KeyID: "k2", PrivateKey: "bb"}, {KeyID: "k3", PrivateKey: "cc"}}
	if len(keys) != len(want) {
		t.Fatalf("keys = %+v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %+v, want %+v", i, keys[i], want[i])
		}
	}

	t.Setenv("HARUKI_TEST_NOISE_KEYS", `[{"key_id":"json1","private_key":"dd"},{"key_id":"json2","private_key":"ee"}]`)
	if err := envNoiseKeys("HARUKI_TEST_NOISE_KEYS", &keys); err != nil {
		t.Fatalf("envNoiseKeys json: %v", err)
	}
	if len(keys) != 2 || keys[0].KeyID != "json1" || keys[1].PrivateKey != "ee" {
		t.Fatalf("json keys = %+v", keys)
	}

	t.Setenv("HARUKI_TEST_NOISE_KEYS", "")
	untouched := []NoiseStaticKeyConfig{{KeyID: "keep", PrivateKey: "ff"}}
	if err := envNoiseKeys("HARUKI_TEST_NOISE_KEYS", &untouched); err != nil || len(untouched) != 1 {
		t.Fatalf("empty env must leave dst untouched: %+v, %v", untouched, err)
	}

	for name, value := range map[string]string{
		"missing separator": "k1",
		"empty id":          "=aa",
		"empty key":         "k1=",
		"bad json":          "[{",
		"json empty id":     `[{"key_id":"","private_key":"aa"}]`,
	} {
		t.Setenv("HARUKI_TEST_NOISE_KEYS", value)
		if err := envNoiseKeys("HARUKI_TEST_NOISE_KEYS", &keys); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}
