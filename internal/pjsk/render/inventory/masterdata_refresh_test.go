package inventory

import (
	"os"
	"path/filepath"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestControllerResetMasterdataCacheReloadsInventoryFiles(t *testing.T) {
	root := t.TempDir()
	masterDir := filepath.Join(root, "haruki-sekai-master", "master")
	if err := os.MkdirAll(masterDir, 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	path := filepath.Join(masterDir, "materials.json")
	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"Old"}]`), 0o644); err != nil {
		t.Fatalf("write initial materials: %v", err)
	}
	controller := &Controller{masterdata: newMasterdataStore(root)}
	if got := controller.masterdata.forRegion(renderregion.JP).materials[1].Name; got != "Old" {
		t.Fatalf("initial material name = %q", got)
	}

	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"Updated"}]`), 0o644); err != nil {
		t.Fatalf("write updated materials: %v", err)
	}
	controller.ResetMasterdataCache()

	if got := controller.masterdata.forRegion(renderregion.JP).materials[1].Name; got != "Updated" {
		t.Fatalf("reloaded material name = %q", got)
	}
}
