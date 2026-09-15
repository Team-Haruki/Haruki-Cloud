package mysekai

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/core/urlhost"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/storage"
)

const (
	housingCompetitionBannerCacheDirName = "mysekai_housing_competition_banners"
	housingCompetitionBannerMaxBytes     = 8 << 20
	housingCompetitionBannerHTTPTimeout  = 10 * time.Second
)

type housingCompetitionBannerCache struct {
	mu         sync.Mutex
	store      storage.Store
	reader     *assets.AssetReader
	hosts      *urlhost.Set
	httpClient *http.Client
	synced     map[string]struct{}
}

// newHousingCompetitionBannerCache keeps fetched banners in store under
// mysekai_housing_competition_banners/ (nil store -> no cache, every read goes
// to the source).
func newHousingCompetitionBannerCache(store storage.Store, reader *assets.AssetReader, hosts *urlhost.Set) *housingCompetitionBannerCache {
	return &housingCompetitionBannerCache{
		store:      store,
		reader:     reader,
		hosts:      hosts,
		httpClient: &http.Client{Timeout: housingCompetitionBannerHTTPTimeout},
		synced:     make(map[string]struct{}),
	}
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

	cacheKey := c.cacheKey(imagePath)
	if cacheKey != "" {
		finishLookup := commandtrace.MeasureOperation(ctx, "housing_banner.cache_lookup")
		if raw, err := c.store.Get(ctx, cacheKey); err == nil && len(raw) > 0 {
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
	if cacheKey != "" {
		finishStore := commandtrace.MeasureOperation(ctx, "housing_banner.store")
		_ = c.store.Put(context.WithoutCancel(ctx), cacheKey, raw, storage.PutOptions{})
		finishStore()
	}
	c.markSynced(imagePath)
	return raw, nil
}

func (c *housingCompetitionBannerCache) readSource(ctx context.Context, imagePath string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("housing competition banner cache is not configured")
	}
	if c.reader != nil {
		finishRead := commandtrace.MeasureOperation(ctx, "housing_banner.asset_read")
		raw, _, err := c.reader.ReadFirst(ctx, imagePath)
		finishRead()
		if !errors.Is(err, storage.ErrNotExist) {
			return raw, err
		}
	}
	return c.downloadSource(ctx, imagePath)
}

// downloadSource fetches the banner from the public asset hosts. A transport
// error or a 5xx response cools the host down and moves on to the next one;
// any other response ends the attempt.
func (c *housingCompetitionBannerCache) downloadSource(ctx context.Context, imagePath string) ([]byte, error) {
	if c == nil || c.hosts.Len() == 0 {
		return nil, fmt.Errorf("housing competition banner not found: %s", imagePath)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for range c.hosts.Len() {
		base := c.hosts.Base("")
		sourceURL, err := assets.PublicAssetURL(base, imagePath)
		if err != nil {
			return nil, fmt.Errorf("invalid housing competition banner path %s: %w", imagePath, err)
		}
		raw, retry, err := c.fetch(ctx, sourceURL)
		if err == nil || !retry {
			return raw, err
		}
		c.markHostFailure(base)
		lastErr = err
	}
	return nil, lastErr
}

func (c *housingCompetitionBannerCache) fetch(ctx context.Context, sourceURL string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, housingCompetitionBannerHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, false, err
	}
	finishHTTP := commandtrace.MeasureOperation(ctx, "housing_banner.http")
	defer finishHTTP()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode >= http.StatusInternalServerError, fmt.Errorf("download housing competition banner failed: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, housingCompetitionBannerMaxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(raw) > housingCompetitionBannerMaxBytes {
		return nil, false, fmt.Errorf("housing competition banner is too large")
	}
	return raw, false, nil
}

func (c *housingCompetitionBannerCache) markHostFailure(base string) {
	for _, host := range c.hosts.Hosts() {
		if host.BaseURL == base {
			c.hosts.MarkFailure(host.Name)
			return
		}
	}
}

// cacheKey returns the store key of imagePath's cached copy, or "" when the
// cache is off or the path cannot name a key.
func (c *housingCompetitionBannerCache) cacheKey(imagePath string) storage.Key {
	if c == nil || c.store == nil {
		return ""
	}
	rel := housingCompetitionBannerCacheRelPath(imagePath)
	if rel == "" {
		return ""
	}
	key, err := storage.Join(housingCompetitionBannerCacheDirName, rel)
	if err != nil {
		return ""
	}
	return key
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
