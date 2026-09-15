package assets

import (
	"errors"
	"testing"

	"haruki-cloud/internal/storage"
)

func TestObjectKeyStripTable(t *testing.T) {
	cases := []struct {
		in   string
		want storage.Key
	}{
		{"asset/jp-assets/startapp/x", "jp-assets/startapp/x"},
		{"/asset/jp-assets/startapp/x", "jp-assets/startapp/x"},
		{"jp-assets/startapp/x", "jp-assets/startapp/x"},
		{"/local/root/jp-assets/startapp/x", "jp-assets/startapp/x"},
		{"C:\\srv\\assets\\cn-assets\\ondemand\\y.png", "cn-assets/ondemand/y.png"},
		{"  asset/tw-assets/startapp/a b.png ", "tw-assets/startapp/a b.png"},
		{"static_images/pjsk_3d_preview/1.png", "static_images/pjsk_3d_preview/1.png"},
		{"/static_images/pjsk_3d_preview/1.png", "static_images/pjsk_3d_preview/1.png"},
		{"asset/static_images/x.png", "static_images/x.png"},
		{"./stamp/a/a.png", "stamp/a/a.png"},
		{"-assets/x.png", "-assets/x.png"},
		{"asset/jp-assets/./startapp/x", "jp-assets/startapp/x"},
	}
	for _, tc := range cases {
		got, err := ObjectKey(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ObjectKey(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestObjectKeyRejects(t *testing.T) {
	for _, in := range []string{"", "   ", "/", "asset/", "../etc/passwd", "a/../jp-assets/x", "jp-assets/../x", "jp-assets/x\x01", "a//b"} {
		if got, err := ObjectKey(in); !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("ObjectKey(%q) = %q, %v; want ErrInvalidKey", in, got, err)
		}
	}
}

// TestPublicAssetURLTable pins the cloud-plan §3.1 table, including the two
// accepted behaviour changes (addendum C-2): the rule-A local-root prefix loss
// and the rule-B "../" error.
func TestPublicAssetURLTable(t *testing.T) {
	const base = "https://assets.example"
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"asset prefix", "asset/jp-assets/startapp/x.png", base + "/jp-assets/startapp/x.png", false},
		{"slash asset prefix", "/asset/jp-assets/startapp/x.png", base + "/jp-assets/startapp/x.png", false},
		{"bare region", "jp-assets/startapp/x.png", base + "/jp-assets/startapp/x.png", false},
		{"local root (rule A change)", "/srv/assets/jp-assets/startapp/x.png", base + "/jp-assets/startapp/x.png", false},
		{"static images", "static_images/pjsk_3d_preview/1.png", base + "/static_images/pjsk_3d_preview/1.png", false},
		{"traversal (rule B change)", "../etc/passwd", "", true},
		{"passthrough https", "https://cdn/x.png", "https://cdn/x.png", false},
		{"passthrough http", " http://cdn/x.png ", "http://cdn/x.png", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PublicAssetURL(base+"/", tc.in)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("PublicAssetURL(%q) = %q, %v; want %q (err=%v)", tc.in, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestPublicAssetURLEscapesSegments(t *testing.T) {
	got, err := PublicAssetURL("https://assets.example/base", "asset/jp-assets/startapp/a b/c?d#e%/ハル.png")
	want := "https://assets.example/base/jp-assets/startapp/a%20b/c%3Fd%23e%25/%E3%83%8F%E3%83%AB.png"
	if err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}

func TestPublicAssetURLEmptyBase(t *testing.T) {
	if _, err := PublicAssetURL("  /", "jp-assets/x.png"); !errors.Is(err, ErrNoAssetBaseURL) {
		t.Fatalf("err = %v", err)
	}
	if got, err := PublicAssetURL("", "https://cdn/x.png"); err != nil || got != "https://cdn/x.png" {
		t.Fatalf("passthrough without base = %q, %v", got, err)
	}
}
