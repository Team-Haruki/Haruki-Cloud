package accountdata

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/storage"
)

const (
	DefaultProfileBGRelativeDir = "user_upload/profile_bg"
	defaultProfileBGBlur        = 1
	defaultProfileBGAlpha       = 50
	maxProfileBGSizeBytes       = 1 * 1024 * 1024 // 1 MB
	// maxProfileBGDownloadBytes caps the raw source download before decode, so a
	// hostile server cannot stream an unbounded body into memory.
	maxProfileBGDownloadBytes = 16 * 1024 * 1024 // 16 MB
	// maxProfileBGPixels rejects pixel bombs (small compressed file declaring huge
	// dimensions) before image.Decode allocates the pixel buffer. ~24 MP allows
	// real phone photos while blocking e.g. a 30000x30000 (=900 MP) bomb.
	maxProfileBGPixels   int64 = 24_000_000
	profileBGContentType       = "image/jpeg"
)

// randomHex8 returns 8 random lowercase hex characters for use in filenames.
func randomHex8() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// flattenToRGB composites src onto a white background so that transparent
// pixels become white rather than black when JPEG-encoding an RGBA image.
func flattenToRGB(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, b, src, b.Min, draw.Over)
	return dst
}

// encodeJPEGCompressedContext encodes img as JPEG, starting at quality 92 and
// reducing in steps (80 → 70 → 60) until the output is ≤ maxProfileBGSizeBytes.
// Transparency is flattened onto white before encoding.
func encodeJPEGCompressedContext(ctx context.Context, img image.Image) ([]byte, error) {
	ctx = profileBGContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img = flattenToRGB(img)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, q := range []int{92, 80, 70, 60} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data := buf.Bytes()
		if len(data) <= maxProfileBGSizeBytes || q == 60 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("无法压缩图片至 1MB 以下")
}

// decodeBoundedImage decodes raw image bytes, rejecting "pixel bombs": it first
// reads only the header via image.DecodeConfig and refuses to allocate the pixel
// buffer when width*height exceeds maxPixels. maxPixels <= 0 disables the check.
func decodeBoundedImage(raw []byte, maxPixels int64) (image.Image, error) {
	return decodeBoundedImageContext(context.Background(), raw, maxPixels)
}

func decodeBoundedImageContext(ctx context.Context, raw []byte, maxPixels int64) (image.Image, error) {
	ctx = profileBGContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("背景图片数据为空")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解析背景图片失败: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("解析背景图片失败: 无效的图片尺寸")
	}
	if maxPixels > 0 && int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("背景图片尺寸过大（上限 %d 像素）", maxPixels)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解析背景图片失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return img, nil
}

func profileBGContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// ProfileBGStore saves and deletes custom profile backgrounds through a
// storage.Store (the user_upload slot). Objects are keyed by the persisted
// ProfileBgSettings.ImgPath ("user_upload/profile_bg/<server>/uid_*.jpg").
type ProfileBGStore struct {
	store       storage.Store
	relativeDir string
	client      *http.Client
}

// NewProfileBGStore returns a profile background store over store. A nil store
// is treated as storage.Disabled(), which reports "not configured" on save.
func NewProfileBGStore(store storage.Store) *ProfileBGStore {
	if store == nil {
		store = storage.Disabled()
	}
	return &ProfileBGStore{
		store:       store,
		relativeDir: DefaultProfileBGRelativeDir,
		// SSRF-safe client: refuses to connect to non-public addresses (incl. on
		// redirects) so an attacker-supplied image_url cannot reach loopback /
		// internal / cloud-metadata hosts.
		client: newSSRFSafeClient(config.ProfileBGStoreTimeout),
	}
}

// NewLocalProfileBGStore returns a store rooted at the local directory rootDir
// (a relative directory resolves against the working directory, as before). A
// blank or unusable root yields nil, which reports "not configured".
func NewLocalProfileBGStore(rootDir string) *ProfileBGStore {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil
	}
	local, err := storage.NewLocalAt(rootDir, 0)
	if err != nil {
		return nil
	}
	return NewProfileBGStore(local)
}

// NewLocalProfileBGStoreWithClient is like NewLocalProfileBGStore but uses the
// supplied HTTP client. It exists for tests that must fetch from a loopback
// server (which the production SSRF-safe client deliberately blocks). Production
// code MUST use NewLocalProfileBGStore or NewProfileBGStore.
func NewLocalProfileBGStoreWithClient(rootDir string, client *http.Client) *ProfileBGStore {
	s := NewLocalProfileBGStore(rootDir)
	if s != nil && client != nil {
		s.client = client
	}
	return s
}

func (s *ProfileBGStore) configured() bool {
	return s != nil && s.store != nil && s.store != storage.Disabled()
}

func (s *ProfileBGStore) SaveProfileBackground(ctx context.Context, server string, userID string, imageURL string) (*drawing.ProfileBgSettings, error) {
	if !s.configured() {
		return nil, errProfileBGNotConfigured(storage.ErrNotConfigured)
	}
	ctx = profileBGContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return nil, fmt.Errorf("请提供个人信息背景图片")
	}
	parsedURL, err := url.ParseRequestURI(imageURL)
	if err != nil {
		return nil, fmt.Errorf("无效的背景图片地址: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("背景图片地址协议不被允许: %s", parsedURL.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	finishDownload := commandtrace.MeasureOperation(ctx, "profile_bg.download")
	resp, err := s.client.Do(req)
	finishDownload()
	if err != nil {
		return nil, fmt.Errorf("下载背景图片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载背景图片失败: HTTP %d", resp.StatusCode)
	}

	// Cap the raw download, then reject pixel bombs via a header-only dimension
	// check before image.Decode allocates the full pixel buffer.
	finishRead := commandtrace.MeasureOperation(ctx, "profile_bg.read")
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProfileBGDownloadBytes+1))
	finishRead()
	if err != nil {
		return nil, fmt.Errorf("下载背景图片失败: %w", err)
	}
	if len(raw) > maxProfileBGDownloadBytes {
		return nil, fmt.Errorf("背景图片过大（上限 %d MB）", maxProfileBGDownloadBytes/(1024*1024))
	}
	finishDecode := commandtrace.MeasureOperation(ctx, "profile_bg.decode")
	img, err := decodeBoundedImageContext(ctx, raw, maxProfileBGPixels)
	finishDecode()
	if err != nil {
		return nil, err
	}

	finishEncode := commandtrace.MeasureOperation(ctx, "profile_bg.encode")
	data, err := encodeJPEGCompressedContext(ctx, img)
	finishEncode()
	if err != nil {
		return nil, fmt.Errorf("编码背景图片失败: %w", err)
	}

	server = strings.TrimSpace(strings.ToLower(server))
	userID = strings.TrimSpace(userID)
	filename := fmt.Sprintf("uid_%s_%s.jpg", userID, randomHex8())
	relativePath := path.Join(s.relativeDir, server, filename)
	finishStore := commandtrace.MeasureOperation(ctx, "profile_bg.store")
	err = s.put(ctx, relativePath, data)
	finishStore()
	if err != nil {
		return nil, err
	}

	return &drawing.ProfileBgSettings{
		ImgPath:  &relativePath,
		Blur:     defaultProfileBGBlur,
		Alpha:    defaultProfileBGAlpha,
		Vertical: img.Bounds().Dy() > img.Bounds().Dx(),
	}, nil
}

func (s *ProfileBGStore) put(ctx context.Context, relativePath string, data []byte) error {
	key, err := profileBGKey(relativePath)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = s.store.Put(ctx, key, data, storage.PutOptions{ContentType: profileBGContentType})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, storage.ErrNotConfigured):
		return errProfileBGNotConfigured(err)
	default:
		return fmt.Errorf("写入背景图片失败: %w", err)
	}
}

func (s *ProfileBGStore) DeleteProfileBackground(ctx context.Context, settings *drawing.ProfileBgSettings) error {
	ctx = profileBGContext(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s == nil || settings == nil || settings.ImgPath == nil {
		return nil
	}
	key, err := profileBGKey(*settings.ImgPath)
	if err != nil {
		return err
	}
	if !s.configured() {
		return nil
	}
	finishStore := commandtrace.MeasureOperation(ctx, "profile_bg.store")
	err = s.store.Delete(ctx, key)
	finishStore()
	if err != nil && !errors.Is(err, storage.ErrNotExist) {
		return fmt.Errorf("删除背景图片失败: %w", err)
	}
	return nil
}

// profileBGKey turns a persisted relative image path into a storage key,
// rejecting absolute paths and parent-directory escapes.
func profileBGKey(relativePath string) (storage.Key, error) {
	trimmed := strings.ReplaceAll(strings.TrimSpace(relativePath), "\\", "/")
	if strings.HasPrefix(trimmed, "/") || filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("不允许的背景图片路径: %s", relativePath)
	}
	key, err := storage.CleanKey(path.Clean(trimmed))
	if err != nil {
		return "", fmt.Errorf("不允许的背景图片路径: %s", relativePath)
	}
	return key, nil
}

func errProfileBGNotConfigured(err error) error {
	return fmt.Errorf("pjsk: profile background storage is not configured: %w", err)
}
