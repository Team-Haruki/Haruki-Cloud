//lint:file-ignore SA1012 Coverage tests intentionally exercise nil-context fallback behavior.

package accountdata

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

type accountCoverageReadCloser struct {
	err error
}

func (r accountCoverageReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (accountCoverageReadCloser) Close() error               { return nil }

type accountCoverageZeroReader struct{}

func (accountCoverageZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func profileBGResponse(req *http.Request, status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: req}
}

func TestLocalProfileBGStoreValidationDownloadAndDeleteBranches(t *testing.T) {
	ctx := context.Background()
	testLocalProfileBGStoreConstruction(t, ctx)
	testLocalProfileBGDownloadErrors(t, ctx)
	testLocalProfileBGStoragePathErrors(t, ctx)
	testLocalProfileBGDeleteAndPathValidation(t, ctx)
}

func testLocalProfileBGStoreConstruction(t *testing.T, ctx context.Context) {
	t.Helper()
	var nilStore *LocalProfileBGStore
	if NewLocalProfileBGStore(" ") != nil || NewLocalProfileBGStoreWithClient(" ", http.DefaultClient) != nil {
		t.Fatal("blank profile background root should be rejected")
	}
	if _, err := nilStore.SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil {
		t.Fatal("nil profile background store should fail save")
	}
	if err := nilStore.DeleteProfileBackground(ctx, nil); err != nil {
		t.Fatalf("nil profile background store delete = %v", err)
	}
	store := NewLocalProfileBGStore(t.TempDir())
	if store == nil || store.client == nil || store.relativeDir != DefaultProfileBGRelativeDir {
		t.Fatalf("NewLocalProfileBGStore() = %+v", store)
	}
	originalClient := store.client
	if got := NewLocalProfileBGStoreWithClient(t.TempDir(), nil); got == nil || got.client == nil {
		t.Fatal("nil injected client should keep the safe client")
	}
	if originalClient == nil {
		t.Fatal("production profile background client is nil")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.SaveProfileBackground(canceled, "jp", "1", "https://example.test/a.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled save = %v", err)
	}
	for _, rawURL := range []string{"", "https://exa mple.test/a.png", "ftp://example.test/a.png"} {
		if _, err := store.SaveProfileBackground(ctx, "jp", "1", rawURL); err == nil {
			t.Fatalf("SaveProfileBackground(%q) should fail", rawURL)
		}
	}
}

func testLocalProfileBGDownloadErrors(t *testing.T, ctx context.Context) {
	t.Helper()
	transportError := errors.New("round trip failed")
	errorClient := &http.Client{Transport: profileBGRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportError
	})}
	if _, err := NewLocalProfileBGStoreWithClient(t.TempDir(), errorClient).SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil || !strings.Contains(err.Error(), transportError.Error()) {
		t.Fatalf("download transport error = %v", err)
	}
	statusClient := &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return profileBGResponse(req, http.StatusTeapot, io.NopCloser(strings.NewReader("no"))), nil
	})}
	if _, err := NewLocalProfileBGStoreWithClient(t.TempDir(), statusClient).SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil {
		t.Fatal("non-200 background response should fail")
	}
	readClient := &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return profileBGResponse(req, http.StatusOK, accountCoverageReadCloser{err: errors.New("read failed")}), nil
	})}
	if _, err := NewLocalProfileBGStoreWithClient(t.TempDir(), readClient).SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("background read error = %v", err)
	}
	largeClient := &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(io.LimitReader(accountCoverageZeroReader{}, maxProfileBGDownloadBytes+1))
		return profileBGResponse(req, http.StatusOK, body), nil
	})}
	if _, err := NewLocalProfileBGStoreWithClient(t.TempDir(), largeClient).SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("oversized background error = %v", err)
	}
	invalidImageClient := &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return profileBGResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("not-an-image"))), nil
	})}
	if _, err := NewLocalProfileBGStoreWithClient(t.TempDir(), invalidImageClient).SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil {
		t.Fatal("invalid image body should fail")
	}
}

func testLocalProfileBGStoragePathErrors(t *testing.T, ctx context.Context) {
	t.Helper()
	validRaw := pngBytes(t, 3, 7)
	validClient := &http.Client{Transport: profileBGRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return profileBGResponse(req, http.StatusOK, io.NopCloser(bytes.NewReader(validRaw))), nil
	})}
	escapeStore := NewLocalProfileBGStoreWithClient(t.TempDir(), validClient)
	escapeStore.relativeDir = "../escape"
	if _, err := escapeStore.SaveProfileBackground(ctx, "jp", " 1 ", "https://example.test/a.png"); err == nil {
		t.Fatal("escaping profile background relative directory should fail")
	}
	rootFile := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(rootFile, []byte("file"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	fileRootStore := NewLocalProfileBGStoreWithClient(rootFile, validClient)
	if _, err := fileRootStore.SaveProfileBackground(ctx, "jp", "1", "https://example.test/a.png"); err == nil {
		t.Fatal("profile background store rooted at a file should fail")
	}
}

func testLocalProfileBGDeleteAndPathValidation(t *testing.T, ctx context.Context) {
	t.Helper()
	store := NewLocalProfileBGStore(t.TempDir())
	testLocalProfileBGDeleteBranches(t, ctx, store)
	testLocalProfileBGAbsolutePathValidation(t, store)
}

func testLocalProfileBGDeleteBranches(t *testing.T, ctx context.Context, store *LocalProfileBGStore) {
	t.Helper()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.DeleteProfileBackground(canceled, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delete = %v", err)
	}
	if err := store.DeleteProfileBackground(ctx, nil); err != nil || store.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{}) != nil {
		t.Fatal("empty profile background delete should be a no-op")
	}
	escape := "../escape.jpg"
	if err := store.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &escape}); err == nil {
		t.Fatal("escaping profile background delete should fail")
	}
	missing := DefaultProfileBGRelativeDir + "/jp/missing.jpg"
	if err := store.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &missing}); err != nil {
		t.Fatalf("missing profile background delete = %v", err)
	}
	directory := filepath.Join(store.rootDir, DefaultProfileBGRelativeDir, "jp", "nonempty")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create nonempty directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write directory child: %v", err)
	}
	directoryRel := filepath.ToSlash(filepath.Join(DefaultProfileBGRelativeDir, "jp", "nonempty"))
	if err := store.DeleteProfileBackground(ctx, &drawing.ProfileBgSettings{ImgPath: &directoryRel}); err == nil {
		t.Fatal("deleting a nonempty directory as a background should fail")
	}
	fileRel := filepath.ToSlash(filepath.Join(DefaultProfileBGRelativeDir, "jp", "delete.jpg"))
	fileAbs := filepath.Join(store.rootDir, filepath.FromSlash(fileRel))
	if err := os.MkdirAll(filepath.Dir(fileAbs), 0o755); err != nil {
		t.Fatalf("create delete directory: %v", err)
	}
	if err := os.WriteFile(fileAbs, []byte("x"), 0o644); err != nil {
		t.Fatalf("write delete target: %v", err)
	}
	if err := store.DeleteProfileBackground(nil, &drawing.ProfileBgSettings{ImgPath: &fileRel}); err != nil {
		t.Fatalf("delete profile background: %v", err)
	}
}

func testLocalProfileBGAbsolutePathValidation(t *testing.T, store *LocalProfileBGStore) {
	t.Helper()
	for _, relative := range []string{"", ".", "../x", "/absolute.jpg"} {
		if _, err := store.absolutePath(relative); err == nil {
			t.Fatalf("absolutePath(%q) should fail", relative)
		}
	}
	if got, err := store.absolutePath("user_upload/profile_bg/jp/good.jpg"); err != nil || !strings.HasPrefix(got, filepath.Clean(store.rootDir)+string(filepath.Separator)) {
		t.Fatalf("safe absolute path = %q, %v", got, err)
	}
}

func TestProfileBGImageEncodingCleanupAndSafeHTTPBranches(t *testing.T) {
	testProfileBGImageEncoding(t)
	testProfileBGOrphanCleanup(t)
	testProfileBGSafeDial(t)
	testProfileBGSafeHTTPRedirects(t)
}

func testProfileBGImageEncoding(t *testing.T) {
	t.Helper()
	if got := randomHex8(); len(got) != 8 || strings.ToLower(got) != got {
		t.Fatalf("randomHex8() = %q", got)
	}
	transparent := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	flattened := flattenToRGB(transparent)
	r, g, b, _ := flattened.At(0, 0).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("transparent pixel flattened to %#v, want white", flattened.At(0, 0))
	}
	transparent.Set(1, 1, color.NRGBA{R: 255, A: 255})
	data, err := encodeJPEGCompressedContext(nil, transparent)
	if err != nil || len(data) == 0 {
		t.Fatalf("encodeJPEGCompressedContext() = %d bytes, %v", len(data), err)
	}
	if _, err := decodeBoundedImageContext(nil, nil, 10); err == nil {
		t.Fatal("empty bounded image should fail")
	}
	if _, err := decodeBoundedImageContext(nil, []byte("garbage"), 10); err == nil {
		t.Fatal("invalid bounded image should fail")
	}
	if got := profileBGContext(nil); got == nil || got.Err() != nil {
		t.Fatal("nil profile background context should become a live context")
	}
}

func testProfileBGOrphanCleanup(t *testing.T) {
	t.Helper()
	var nilStore *LocalProfileBGStore
	if deleted, err := nilStore.CleanupOrphanedFiles(context.Background(), nil); err != nil || deleted != 0 {
		t.Fatalf("nil cleanup = %d, %v", deleted, err)
	}
	root := t.TempDir()
	store := NewLocalProfileBGStore(root)
	if deleted, err := store.CleanupOrphanedFiles(context.Background(), nil); err != nil || deleted != 0 {
		t.Fatalf("cleanup missing directory = %d, %v", deleted, err)
	}
	activeRel := filepath.ToSlash(filepath.Join(DefaultProfileBGRelativeDir, "jp", "active.jpg"))
	orphanRel := filepath.ToSlash(filepath.Join(DefaultProfileBGRelativeDir, "jp", "orphan.jpg"))
	for _, rel := range []string{activeRel, orphanRel} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create cleanup directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write cleanup file: %v", err)
		}
	}
	deleted, err := store.CleanupOrphanedFiles(context.Background(), map[string]bool{activeRel: true})
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup orphan = %d, %v", deleted, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(activeRel))); err != nil {
		t.Fatalf("active background was removed: %v", err)
	}
	badRoot := filepath.Join(t.TempDir(), "file-root")
	if err := os.WriteFile(badRoot, []byte("x"), 0o644); err != nil {
		t.Fatalf("write bad cleanup root: %v", err)
	}
	badStore := &LocalProfileBGStore{rootDir: badRoot, relativeDir: DefaultProfileBGRelativeDir}
	if deleted, err := badStore.CleanupOrphanedFiles(context.Background(), nil); err != nil || deleted != 0 {
		t.Fatalf("cleanup below a file root should skip the walk error: %d, %v", deleted, err)
	}
}

func testProfileBGSafeDial(t *testing.T) {
	t.Helper()
	dial := safeDialContext(&net.Dialer{Timeout: time.Millisecond})
	if _, err := dial(context.Background(), "tcp", "missing-port"); err == nil {
		t.Fatal("dial address without a port should fail")
	}
	if _, err := dial(context.Background(), "tcp", "127.0.0.1:80"); !errors.Is(err, errBlockedAddress) {
		t.Fatalf("blocked literal dial = %v", err)
	}
	if _, err := dial(context.Background(), "tcp", "localhost:80"); !errors.Is(err, errBlockedAddress) {
		t.Fatalf("blocked hostname dial = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dial(canceled, "tcp", "8.8.8.8:53"); err == nil {
		t.Fatal("canceled public literal dial should fail")
	}
}

func testProfileBGSafeHTTPRedirects(t *testing.T) {
	t.Helper()
	client := newSSRFSafeClient(123 * time.Millisecond)
	if client.Timeout != 123*time.Millisecond || client.Transport == nil || client.CheckRedirect == nil {
		t.Fatalf("safe HTTP client = %+v", client)
	}
	req := &http.Request{URL: &url.URL{Scheme: "http"}}
	if err := client.CheckRedirect(req, make([]*http.Request, 5)); err == nil {
		t.Fatal("redirect chain longer than five should fail")
	}
	req.URL.Scheme = "file"
	if err := client.CheckRedirect(req, nil); err == nil {
		t.Fatal("non-HTTP redirect should fail")
	}
	req.URL.Scheme = "https"
	if err := client.CheckRedirect(req, nil); err != nil {
		t.Fatalf("HTTPS redirect should succeed: %v", err)
	}
}
