package imagecache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactURLUsesCurrentBaseAndRejectsEscapingFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "image #1.png")
	if err := os.WriteFile(file, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"https://one.test", "https://two.test/cdn"} {
		client := New(base, root)
		url, ok := client.URLForFile(t.Context(), file)
		if !ok || url != base+"/image%20%231.png" {
			t.Fatalf("url=%q ok=%v", url, ok)
		}
	}
	client := New("https://one.test", root)
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, link, root, filepath.Join(root, "absent.png")} {
		if url, ok := client.URLForFile(t.Context(), path); ok {
			t.Fatalf("accepted %s as %s", path, url)
		}
	}
}
