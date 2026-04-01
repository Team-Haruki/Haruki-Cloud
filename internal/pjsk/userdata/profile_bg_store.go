package userdata

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"haruki-cloud/config"
	"haruki-cloud/utils/drawing"
)

const (
	DefaultProfileBGRelativeDir = "user_upload/profile_bg"
	defaultProfileBGBlur        = 4
	defaultProfileBGAlpha       = 80
)

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

func (s *LocalProfileBGStore) SaveProfileBackground(ctx context.Context, server string, bindingID int, imageURL string) (*drawing.ProfileBgSettings, error) {
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

	server = strings.TrimSpace(strings.ToLower(server))
	relativePath := filepath.ToSlash(filepath.Join(s.relativeDir, server, fmt.Sprintf("binding_%d.jpg", bindingID)))
	absolutePath, err := s.absolutePath(relativePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return nil, fmt.Errorf("创建背景目录失败: %w", err)
	}

	file, err := os.Create(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("写入背景图片失败: %w", err)
	}
	defer file.Close()

	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("编码背景图片失败: %w", err)
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
