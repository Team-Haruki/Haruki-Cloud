package accountdata

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
	"haruki-cloud/internal/storage/storagetest"
	"haruki-cloud/utils/logger"
)

type profileBGFailingStore struct {
	storage.Store
	putErr    error
	deleteErr error
}

func (s profileBGFailingStore) Put(ctx context.Context, key storage.Key, data []byte, opts storage.PutOptions) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.Store.Put(ctx, key, data, opts)
}

func (s profileBGFailingStore) Delete(ctx context.Context, key storage.Key) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.Store.Delete(ctx, key)
}

func profileBGImageClient(raw []byte) *http.Client {
	return &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return profileBGResponse(req, http.StatusOK, io.NopCloser(bytes.NewReader(raw))), nil
	})}
}

func profileBGStoreWith(store storage.Store, raw []byte) *ProfileBGStore {
	bg := NewProfileBGStore(store)
	bg.client = profileBGImageClient(raw)
	return bg
}

// legacyProfileBGWrite is the pre-storage implementation of the write step
// (MkdirAll + WriteFile at <root>/<relativePath>, mode 0644).
func legacyProfileBGWrite(t *testing.T, root, relativePath string, data []byte) string {
	t.Helper()
	target := filepath.Join(filepath.Clean(root), filepath.Clean(relativePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("legacy mkdir: %v", err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatalf("legacy write: %v", err)
	}
	return target
}

func TestProfileBGSaveThroughStore(t *testing.T) {
	ctx := context.Background()
	raw := pngBytes(t, 12, 30)
	img, err := decodeBoundedImageContext(ctx, raw, maxProfileBGPixels)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	expected, err := encodeJPEGCompressedContext(ctx, img)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	root := t.TempDir()
	settings, err := NewLocalProfileBGStoreWithClient(root, profileBGImageClient(raw)).SaveProfileBackground(ctx, " JP ", " 42 ", "https://example.test/bg.png")
	if err != nil {
		t.Fatalf("SaveProfileBackground() error = %v", err)
	}
	rel := *settings.ImgPath
	if !strings.HasPrefix(rel, "user_upload/profile_bg/jp/uid_42_") || !strings.HasSuffix(rel, ".jpg") {
		t.Fatalf("ImgPath = %q", rel)
	}
	if !settings.Vertical || settings.Blur != defaultProfileBGBlur || settings.Alpha != defaultProfileBGAlpha {
		t.Fatalf("settings = %+v", settings)
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	stored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read stored background: %v", err)
	}
	legacyTarget := legacyProfileBGWrite(t, t.TempDir(), rel, expected)
	legacy, err := os.ReadFile(legacyTarget)
	if err != nil {
		t.Fatalf("read legacy background: %v", err)
	}
	if !bytes.Equal(stored, legacy) {
		t.Fatal("store output differs from the pre-change implementation")
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{target, legacyTarget} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o644 {
				t.Fatalf("mode of %s = %v, %v", path, info.Mode().Perm(), err)
			}
		}
	}

	memory := storagetest.NewMemory()
	settings, err = profileBGStoreWith(memory, raw).SaveProfileBackground(ctx, "en", "7", "https://example.test/bg.png")
	if err != nil {
		t.Fatalf("memory save: %v", err)
	}
	data, err := memory.Get(ctx, storage.Key(*settings.ImgPath))
	if err != nil || !bytes.Equal(data, expected) {
		t.Fatalf("memory object = %d bytes, %v", len(data), err)
	}
	if err := profileBGStoreWith(memory, raw).DeleteProfileBackground(ctx, settings); err != nil {
		t.Fatalf("memory delete: %v", err)
	}
	if _, err := memory.Get(ctx, storage.Key(*settings.ImgPath)); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("deleted object still readable: %v", err)
	}
}

func TestProfileBGStoreDisabledReportsNotConfigured(t *testing.T) {
	ctx := context.Background()
	for name, store := range map[string]*ProfileBGStore{
		"nil store":      NewProfileBGStore(nil),
		"disabled store": NewProfileBGStore(storage.Disabled()),
		"nil pointer":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := store.SaveProfileBackground(ctx, "jp", "1", "https://example.test/bg.png")
			if !errors.Is(err, storage.ErrNotConfigured) || !strings.Contains(err.Error(), "profile background storage is not configured") {
				t.Fatalf("save error = %v", err)
			}
			rel := DefaultProfileBGRelativeDir + "/jp/x.jpg"
			if err := store.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &rel}); err != nil {
				t.Fatalf("delete on unconfigured store = %v", err)
			}
		})
	}
	if NewLocalProfileBGStore("") != nil {
		t.Fatal("blank local root should be unconfigured")
	}

	// A store that only reports ErrNotConfigured at write time still maps to
	// the not-configured error.
	raw := pngBytes(t, 2, 2)
	lateDisabled := profileBGStoreWith(profileBGFailingStore{Store: storagetest.NewMemory(), putErr: storage.ErrNotConfigured}, raw)
	if _, err := lateDisabled.SaveProfileBackground(ctx, "jp", "1", "https://example.test/bg.png"); !errors.Is(err, storage.ErrNotConfigured) || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("late not-configured error = %v", err)
	}
}

func TestProfileBGStoreFailuresSurface(t *testing.T) {
	ctx := context.Background()
	raw := pngBytes(t, 2, 2)
	putErr := errors.New("put exploded")
	failing := profileBGStoreWith(profileBGFailingStore{Store: storagetest.NewMemory(), putErr: putErr}, raw)
	if _, err := failing.SaveProfileBackground(ctx, "jp", "1", "https://example.test/bg.png"); !errors.Is(err, putErr) || !strings.Contains(err.Error(), "写入背景图片失败") {
		t.Fatalf("put failure = %v", err)
	}

	deleteErr := errors.New("delete exploded")
	rel := DefaultProfileBGRelativeDir + "/jp/x.jpg"
	failingDelete := NewProfileBGStore(profileBGFailingStore{Store: storagetest.NewMemory(), deleteErr: deleteErr})
	if err := failingDelete.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &rel}); !errors.Is(err, deleteErr) || !strings.Contains(err.Error(), "删除背景图片失败") {
		t.Fatalf("delete failure = %v", err)
	}
	notExist := NewProfileBGStore(profileBGFailingStore{Store: storagetest.NewMemory(), deleteErr: storage.NotExistError("delete", "x")})
	if err := notExist.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &rel}); err != nil {
		t.Fatalf("missing object delete = %v", err)
	}

	escape := profileBGStoreWith(storagetest.NewMemory(), raw)
	escape.relativeDir = "../outside"
	if _, err := escape.SaveProfileBackground(ctx, "jp", "1", "https://example.test/bg.png"); err == nil || !strings.Contains(err.Error(), "不允许的背景图片路径") {
		t.Fatalf("escaping key error = %v", err)
	}
}

func TestProfileBGDualWriteLogsMirrorFailure(t *testing.T) {
	ctx := context.Background()
	raw := pngBytes(t, 4, 4)
	primary := storagetest.NewMemory()
	mirror := storagetest.NewMemory()
	mirror.FailPut = func(storage.Key) error { return errors.New("garage unavailable") }
	var logs bytes.Buffer
	dual := storage.NewDual(primary, mirror, storage.DualWrite, logger.NewLogger("storage", "DEBUG", &logs))
	settings, err := profileBGStoreWith(dual, raw).SaveProfileBackground(ctx, "jp", "9", "https://example.test/bg.png")
	if err != nil {
		t.Fatalf("dual save error = %v", err)
	}
	if _, err := primary.Get(ctx, storage.Key(*settings.ImgPath)); err != nil {
		t.Fatalf("primary missing background: %v", err)
	}
	for _, want := range []string{"WARN", "storage mirror put failed", "garage unavailable"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log %q missing %q", logs.String(), want)
		}
	}

	primaryErr := errors.New("primary down")
	primary.FailPut = func(storage.Key) error { return primaryErr }
	if _, err := profileBGStoreWith(dual, raw).SaveProfileBackground(ctx, "jp", "9", "https://example.test/bg.png"); !errors.Is(err, primaryErr) {
		t.Fatalf("primary failure = %v", err)
	}
}
