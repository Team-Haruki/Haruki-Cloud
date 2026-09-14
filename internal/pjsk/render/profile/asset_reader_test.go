package profile

import (
	"testing"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage/storagetest"
)

func TestProfileSetAssetReader(t *testing.T) {
	var nilController *Controller
	nilController.SetAssetReader(nil)

	controller := NewController(nil, nil, assets.NewAssetHelper("", nil), nil)
	reader := assets.NewAssetReader(nil, storagetest.NewMemory())
	controller.SetAssetReader(reader)
	if controller.assetReader != reader {
		t.Fatal("reader not set")
	}
}
