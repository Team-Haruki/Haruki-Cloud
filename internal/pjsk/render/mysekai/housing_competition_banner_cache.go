package mysekai

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/assets"
)

const (
	housingCompetitionBannerCacheDirName = "mysekai_housing_competition_banners"
	housingCompetitionBannerMaxBytes     = 8 << 20
	housingCompetitionBannerHTTPTimeout  = 10 * time.Second
)

type housingCompetitionBannerCache struct {
	mu            sync.Mutex
	dir           string
	assets        *assets.AssetHelper
	assetsBaseURL string
	httpClient    *http.Client
	synced        map[string]struct{}
}

func newHousingCompetitionBannerCache(cacheDir string, assetHelper *assets.AssetHelper, assetsBaseURL string) *housingCompetitionBannerCache {
	return &housingCompetitionBannerCache{
		dir:           strings.TrimSpace(cacheDir),
		assets:        assetHelper,
		assetsBaseURL: strings.TrimRight(strings.TrimSpace(assetsBaseURL), "/"),
		httpClient:    &http.Client{Timeout: housingCompetitionBannerHTTPTimeout},
		synced:        make(map[string]struct{}),
	}
}

func defaultHousingCompetitionBannerCacheDir(statsCachePath string) string {
	statsCachePath = strings.TrimSpace(statsCachePath)
	if statsCachePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(statsCachePath), housingCompetitionBannerCacheDirName)
}

func (c *Controller) syncHousingCompetitionBannersFromMasterdata() {
	if c == nil || c.housingCompetitionBanners == nil || c.masterdata == nil {
		return
	}
	items := c.masterdata.loadList("mysekaiHousingCompetitions.json")
	for _, item := range items {
		info := c.housingCompetitionInfoFromMasterdata(item)
		if info.BackgroundImageAssetbundleFileName == "" {
			continue
		}
		_ = c.housingCompetitionBanners.SyncContext(c.requestCtx, info.BannerImgPath)
	}
}

func (c *Controller) housingCompetitionBannerBase64(info HousingCompetitionInfo) *string {
	if c == nil || c.housingCompetitionBanners == nil {
		return nil
	}
	return c.housingCompetitionBanners.Base64Context(c.requestCtx, info.BannerImgPath)
}

func (c *housingCompetitionBannerCache) Base64(imagePath string) *string {
	return c.Base64Context(context.Background(), imagePath)
}

func (c *housingCompetitionBannerCache) Base64Context(ctx context.Context, imagePath string) *string {
	raw, err := c.BytesContext(ctx, imagePath)
	if err != nil || len(raw) == 0 {
		return nil
	}
	finishEncode := commandtrace.MeasureOperation(ctx, "housing_banner.encode")
	encoded := base64.StdEncoding.EncodeToString(raw)
	finishEncode()
	return &encoded
}

func (c *housingCompetitionBannerCache) Sync(imagePath string) error {
	return c.SyncContext(context.Background(), imagePath)
}

func (c *housingCompetitionBannerCache) SyncContext(ctx context.Context, imagePath string) error {
	if c.isSynced(imagePath) {
		return nil
	}
	_, err := c.BytesContext(ctx, imagePath)
	return err
}

func (c *housingCompetitionBannerCache) Bytes(imagePath string) ([]byte, error) {
	return c.BytesContext(context.Background(), imagePath)
}

func (c *housingCompetitionBannerCache) BytesContext(ctx context.Context, imagePath string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return nil, fmt.Errorf("empty housing competition banner path")
	}
	if c == nil {
		return nil, fmt.Errorf("housing competition banner cache is not configured")
	}

	cachePath := c.cachePath(imagePath)
	if cachePath != "" {
		finishLookup := commandtrace.MeasureOperation(ctx, "housing_banner.cache_lookup")
		if raw, err := os.ReadFile(cachePath); err == nil && len(raw) > 0 {
			finishLookup()
			c.markSynced(imagePath)
			return raw, nil
		}
		finishLookup()
	}

	raw, err := c.readSource(ctx, imagePath)
	if err != nil || len(raw) == 0 {
		return raw, err
	}
	if cachePath != "" {
		finishStore := commandtrace.MeasureOperation(ctx, "housing_banner.store")
		_ = c.write(cachePath, raw)
		finishStore()
	}
	c.markSynced(imagePath)
	return raw, nil
}

func (c *housingCompetitionBannerCache) readSource(ctx context.Context, imagePath string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("housing competition banner cache is not configured")
	}
	if c.assets != nil {
		if resolved := c.assets.WithContext(ctx).FirstExisting(imagePath); resolved != "" {
			finishRead := commandtrace.MeasureOperation(ctx, "housing_banner.asset_read")
			raw, err := os.ReadFile(resolved)
			finishRead()
			return raw, err
		}
	}
	return c.downloadSource(ctx, imagePath)
}

func (c *housingCompetitionBannerCache) downloadSource(ctx context.Context, imagePath string) ([]byte, error) {
	if c == nil || c.assetsBaseURL == "" {
		return nil, fmt.Errorf("housing competition banner not found: %s", imagePath)
	}
	sourceURL, err := c.sourceURL(imagePath)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, housingCompetitionBannerHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	finishHTTP := commandtrace.MeasureOperation(ctx, "housing_banner.http")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		finishHTTP()
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		finishHTTP()
		return nil, fmt.Errorf("download housing competition banner failed: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, housingCompetitionBannerMaxBytes+1))
	if err != nil {
		finishHTTP()
		return nil, err
	}
	if len(raw) > housingCompetitionBannerMaxBytes {
		finishHTTP()
		return nil, fmt.Errorf("housing competition banner is too large")
	}
	finishHTTP()
	return raw, nil
}

func (c *housingCompetitionBannerCache) sourceURL(imagePath string) (string, error) {
	if c == nil || c.assetsBaseURL == "" {
		return "", fmt.Errorf("asset base url is not configured")
	}
	parsed, err := url.Parse(c.assetsBaseURL)
	if err != nil {
		return "", err
	}
	rel := strings.TrimSpace(imagePath)
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	rel = strings.TrimPrefix(rel, "asset/")
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", fmt.Errorf("invalid housing competition banner path: %s", imagePath)
	}
	joined := []string{}
	if parsed.Path != "" {
		joined = append(joined, parsed.Path)
	}
	joined = append(joined, rel)
	parsed.Path = path.Join(joined...)
	return parsed.String(), nil
}

func (c *housingCompetitionBannerCache) cachePath(imagePath string) string {
	if c == nil || strings.TrimSpace(c.dir) == "" {
		return ""
	}
	rel := housingCompetitionBannerCacheRelPath(imagePath)
	if rel == "" {
		return ""
	}
	return filepath.Join(c.dir, filepath.FromSlash(rel))
}

func (c *housingCompetitionBannerCache) write(path string, raw []byte) error {
	if path == "" || len(raw) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func housingCompetitionBannerCacheRelPath(imagePath string) string {
	trimmed := strings.TrimSpace(imagePath)
	isAbs := filepath.IsAbs(trimmed)
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(trimmed, "/")))
	if clean == "" || clean == "." {
		return ""
	}
	if isAbs || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		sum := sha256.Sum256([]byte(imagePath))
		ext := strings.ToLower(filepath.Ext(imagePath))
		if ext == "" {
			ext = ".bin"
		}
		return filepath.ToSlash(filepath.Join("by_hash", hex.EncodeToString(sum[:])+ext))
	}
	return clean
}

func (c *housingCompetitionBannerCache) isSynced(imagePath string) bool {
	if c == nil {
		return false
	}
	key := strings.TrimSpace(imagePath)
	if key == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.synced[key]
	return ok
}

func (c *housingCompetitionBannerCache) markSynced(imagePath string) {
	if c == nil {
		return
	}
	key := strings.TrimSpace(imagePath)
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.synced == nil {
		c.synced = make(map[string]struct{})
	}
	c.synced[key] = struct{}{}
}
