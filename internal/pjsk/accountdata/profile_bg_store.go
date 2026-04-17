package accountdata

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/internal/pjsk/drawing"
)

const (
	DefaultProfileBGRelativeDir = "user_upload/profile_bg"
	defaultProfileBGBlur        = 4
	defaultProfileBGAlpha       = 80
	maxProfileBGSizeBytes       = 1 * 1024 * 1024 // 1 MB
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

// encodeJPEGCompressed encodes img as JPEG, starting at quality 92 and
// reducing in steps (80 → 70 → 60) until the output is ≤ maxProfileBGSizeBytes.
// Transparency is flattened onto white before encoding.
func encodeJPEGCompressed(img image.Image) ([]byte, error) {
	img = flattenToRGB(img)
	for _, q := range []int{92, 80, 70, 60} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		data := buf.Bytes()
		if len(data) <= maxProfileBGSizeBytes || q == 60 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("无法压缩图片至 1MB 以下")
}

type LocalProfileBGStore struct {
	rootDir     string
	relativeDir string
	client      *http.Client
}

func NewLocalProfileBGStore(rootDir string) *LocalProfileBGStore {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil
	}
	return &LocalProfileBGStore{
		rootDir:     rootDir,
		relativeDir: DefaultProfileBGRelativeDir,
		client: &http.Client{
			Timeout: config.ProfileBGStoreTimeout,
		},
	}
}

func (s *LocalProfileBGStore) SaveProfileBackground(ctx context.Context, server string, userID string, imageURL string) (*drawing.ProfileBgSettings, error) {
	if s == nil {
		return nil, fmt.Errorf("pjsk: profile background storage is not configured")
	}
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return nil, fmt.Errorf("请提供个人信息背景图片")
	}
	if _, err := url.ParseRequestURI(imageURL); err != nil {
		return nil, fmt.Errorf("无效的背景图片地址: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载背景图片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载背景图片失败: HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析背景图片失败: %w", err)
	}

	data, err := encodeJPEGCompressed(img)
	if err != nil {
		return nil, fmt.Errorf("编码背景图片失败: %w", err)
	}

	server = strings.TrimSpace(strings.ToLower(server))
	userID = strings.TrimSpace(userID)
	filename := fmt.Sprintf("uid_%s_%s.jpg", userID, randomHex8())
	relativePath := filepath.ToSlash(filepath.Join(s.relativeDir, server, filename))
	absolutePath, err := s.absolutePath(relativePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return nil, fmt.Errorf("创建背景目录失败: %w", err)
	}
	if err := os.WriteFile(absolutePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("写入背景图片失败: %w", err)
	}

	return &drawing.ProfileBgSettings{
		ImgPath:  &relativePath,
		Blur:     defaultProfileBGBlur,
		Alpha:    defaultProfileBGAlpha,
		Vertical: img.Bounds().Dy() > img.Bounds().Dx(),
	}, nil
}

func (s *LocalProfileBGStore) DeleteProfileBackground(ctx context.Context, settings *drawing.ProfileBgSettings) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if s == nil || settings == nil || settings.ImgPath == nil {
		return nil
	}
	absolutePath, err := s.absolutePath(*settings.ImgPath)
	if err != nil {
		return err
	}
	if err := os.Remove(absolutePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除背景图片失败: %w", err)
	}
	return nil
}

func (s *LocalProfileBGStore) absolutePath(relativePath string) (string, error) {
	relativePath = filepath.ToSlash(strings.TrimSpace(relativePath))
	if relativePath == "" {
		return "", fmt.Errorf("无效的背景图片路径")
	}
	cleanRoot := filepath.Clean(s.rootDir)
	cleanRelative := filepath.Clean(relativePath)
	if filepath.IsAbs(cleanRelative) || cleanRelative == "." || strings.HasPrefix(cleanRelative, "..") {
		return "", fmt.Errorf("不允许的背景图片路径: %s", relativePath)
	}
	return filepath.Join(cleanRoot, cleanRelative), nil
}
