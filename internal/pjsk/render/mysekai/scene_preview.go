package mysekai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"

	sonic "github.com/bytedance/sonic"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

const (
	scenePreviewDefaultTileWidth  = 1600
	scenePreviewDefaultTileHeight = 900
	scenePreviewDefaultTimeout    = 2 * time.Minute
)

var scenePreviewMasterdataFiles = []string{
	"mysekaiFixtures.json",
	"mysekaiCustomFixtures.json",
	"mysekaiRankReleases.json",
	"mysekaiSiteLevels.json",
	"mysekaiSiteLayouts.json",
	"cards.json",
	"mysekaiMusicRecords.json",
	"musics.json",
}

type ScenePreviewQuery struct {
	Region             string `json:"region,omitempty"`
	SiteIDs            []int  `json:"site_ids,omitempty"`
	Width              int    `json:"width,omitempty"`
	Height             int    `json:"height,omitempty"`
	BackWallOpacityPct *int   `json:"back_wall_opacity_pct,omitempty"`
}

func (c *Controller) RenderScenePreview(ctx context.Context, query ScenePreviewQuery) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c = c.withRegion(query.Region)
	merged, region, err := c.prepareSnapshot(query.Region)
	if err != nil {
		return nil, err
	}

	layout, err := buildScenePreviewLayout(merged)
	if err != nil {
		return nil, err
	}
	sites, err := normalizeScenePreviewSiteIDs(query.SiteIDs)
	if err != nil {
		return nil, err
	}
	sites = filterScenePreviewAvailableSites(layout, sites)

	width := query.Width
	if width <= 0 {
		width = scenePreviewDefaultTileWidth
	}
	height := query.Height
	if height <= 0 {
		height = scenePreviewDefaultTileHeight
	}
	if width < 640 || width > 3200 || height < 360 || height > 2400 {
		return nil, fmt.Errorf("mysekai scene preview viewport out of range")
	}

	workDir, err := os.MkdirTemp("", "haruki-mysekai-preview-*")
	if err != nil {
		return nil, fmt.Errorf("create mysekai scene preview workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	if err := c.writeScenePreviewWorkspace(workDir, layout); err != nil {
		return nil, err
	}

	server, err := c.newScenePreviewServer(workDir, region.String())
	if err != nil {
		return nil, err
	}
	defer server.Close()

	params := url.Values{}
	params.Set("export", "1")
	params.Set("layout", "/layout.json")
	params.Set("region", region.String())
	params.Set("sites", joinInts(sites))
	params.Set("tileWidth", strconv.Itoa(width))
	params.Set("tileHeight", strconv.Itoa(height))
	params.Set("grid", "0")
	params.Set("shadow", "1")
	if query.BackWallOpacityPct != nil {
		params.Set("backWallOpacity", strconv.Itoa(*query.BackWallOpacityPct))
	} else {
		params.Set("backWallOpacity", "20")
	}

	return captureScenePreview(ctx, server.URL()+"/template.html?"+params.Encode(), width, height*len(sites))
}

func filterScenePreviewAvailableSites(layout map[string]any, siteIDs []int) []int {
	if len(siteIDs) == 0 {
		return siteIDs
	}
	layouts := nestedList(layout, "userMysekaiSiteHousingLayouts")
	if len(layouts) == 0 {
		return siteIDs
	}
	available := make(map[int]struct{}, len(layouts))
	for _, item := range layouts {
		site, ok := item.(map[string]any)
		if !ok {
			continue
		}
		siteID := intNumber(site["mysekaiSiteId"], 0)
		if siteID > 0 {
			available[siteID] = struct{}{}
		}
	}
	if len(available) == 0 {
		return siteIDs
	}
	out := make([]int, 0, len(siteIDs))
	for _, siteID := range siteIDs {
		if _, ok := available[siteID]; ok {
			out = append(out, siteID)
		}
	}
	if len(out) == 0 {
		return siteIDs
	}
	return out
}

func buildScenePreviewLayout(merged map[string]any) (map[string]any, error) {
	if merged == nil {
		return nil, fmt.Errorf("mysekai scene preview contains no data")
	}
	layouts := firstScenePreviewList(merged,
		"userMysekaiSiteHousingLayouts",
		"userMysekaiSiteLayouts",
		"userMysekaiHousingLayouts",
	)
	if len(layouts) == 0 {
		return nil, fmt.Errorf("mysekai scene preview contains no housing layout data")
	}

	out := make(map[string]any, len(merged)+4)
	for key, value := range merged {
		out[key] = value
	}
	out["userMysekaiSiteHousingLayouts"] = layouts

	if _, ok := out["mysekaiRank"]; !ok {
		if rank := scenePreviewNestedInt(merged, "userMysekaiGamedata", "mysekaiRank"); rank > 0 {
			out["mysekaiRank"] = rank
		}
	}
	if _, ok := out["userMysekaiGate"]; !ok {
		if gate := scenePreviewGate(merged); gate != nil {
			out["userMysekaiGate"] = gate
		}
	}
	return out, nil
}

func firstScenePreviewList(root map[string]any, keys ...string) []any {
	for _, key := range keys {
		if items := nestedList(root, key); len(items) > 0 {
			return items
		}
	}
	return nil
}

func scenePreviewGate(root map[string]any) map[string]any {
	if root == nil {
		return nil
	}
	if gate, ok := root["userMysekaiGate"].(map[string]any); ok {
		return gate
	}
	if visit, ok := root["userMysekaiGateCharacterVisit"].(map[string]any); ok {
		if gate, ok := visit["userMysekaiGate"].(map[string]any); ok {
			return gate
		}
	}
	if updated, ok := root["updatedResources"].(map[string]any); ok {
		if gate := scenePreviewGate(updated); gate != nil {
			return gate
		}
	}
	for _, item := range nestedList(root, "userMysekaiGates") {
		if gate, ok := item.(map[string]any); ok && intNumber(gate["mysekaiGateId"], 0) > 0 {
			return gate
		}
	}
	return nil
}

func scenePreviewNestedInt(root map[string]any, parent string, child string) int {
	if value := nestedInt(root, parent, child); value > 0 {
		return value
	}
	if updated, ok := root["updatedResources"].(map[string]any); ok {
		return nestedInt(updated, parent, child)
	}
	return 0
}

func normalizeScenePreviewSiteIDs(siteIDs []int) ([]int, error) {
	if len(siteIDs) == 0 {
		return []int{1, 2, 3, 4}, nil
	}
	out := make([]int, 0, len(siteIDs))
	seen := make(map[int]struct{}, len(siteIDs))
	for _, siteID := range siteIDs {
		if siteID < 1 || siteID > 4 {
			return nil, fmt.Errorf("mysekai scene preview invalid site id: %d", siteID)
		}
		if _, ok := seen[siteID]; ok {
			continue
		}
		seen[siteID] = struct{}{}
		out = append(out, siteID)
	}
	if len(out) == 0 {
		return []int{1, 2, 3, 4}, nil
	}
	return out, nil
}

func (c *Controller) writeScenePreviewWorkspace(root string, layout map[string]any) error {
	if err := os.MkdirAll(filepath.Join(root, "data", "master_data"), 0o755); err != nil {
		return fmt.Errorf("create mysekai scene preview masterdata dir: %w", err)
	}
	if err := writeScenePreviewJSON(filepath.Join(root, "layout.json"), layout); err != nil {
		return err
	}
	if c == nil || c.masterdata == nil || !c.masterdata.Configured() {
		return fmt.Errorf("mysekai scene preview masterdata is not configured")
	}
	for _, filename := range scenePreviewMasterdataFiles {
		items := c.masterdata.loadList(filename)
		if filename == "mysekaiFixtures.json" && len(items) == 0 {
			return fmt.Errorf("mysekai scene preview masterdata missing: %s", filename)
		}
		if len(items) == 0 {
			items = []map[string]any{}
		}
		target := filepath.Join(root, "data", "master_data", filename)
		if err := writeScenePreviewJSON(target, items); err != nil {
			return err
		}
	}
	html := scenePreviewHTML()
	if !strings.Contains(html, "runHeadlessScenePreviewExport") {
		return fmt.Errorf("mysekai scene preview template export bootstrap is not installed")
	}
	return os.WriteFile(filepath.Join(root, "template.html"), []byte(html), 0o644)
}

func writeScenePreviewJSON(path string, value any) error {
	data, err := sonic.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal mysekai scene preview json %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write mysekai scene preview json %s: %w", filepath.Base(path), err)
	}
	return nil
}

type scenePreviewServer struct {
	baseURL string
	server  *http.Server
}

func (s *scenePreviewServer) URL() string {
	if s == nil {
		return ""
	}
	return s.baseURL
}

func (s *scenePreviewServer) Close() {
	if s == nil || s.server == nil {
		return
	}
	_ = s.server.Close()
}

func (c *Controller) newScenePreviewServer(workDir string, region string) (*scenePreviewServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start mysekai scene preview server: %w", err)
	}

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir(workDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/data/assets/") {
			c.serveScenePreviewAsset(w, r, region)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// The caller observes browser-side load failures; no global logger is
			// used here to keep this package independent of app bootstrap.
		}
	}()

	return &scenePreviewServer{
		baseURL: "http://" + listener.Addr().String(),
		server:  server,
	}, nil
}

func (c *Controller) serveScenePreviewAsset(w http.ResponseWriter, r *http.Request, region string) {
	rel := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/"), "data/assets/")
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "/"))
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	resolved := c.resolveScenePreviewAsset(region, rel)
	if resolved == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, resolved)
}

func (c *Controller) resolveScenePreviewAsset(region string, rel string) string {
	if c == nil || c.assets == nil {
		return ""
	}

	candidates := scenePreviewAssetCandidates(region, rel)
	for _, candidate := range candidates {
		if resolved := c.assets.FirstExisting(candidate); resolved != "" {
			return resolved
		}
		if resolved := c.resolveScenePreviewAssetDirectory(candidate); resolved != "" {
			return resolved
		}
	}
	return ""
}

func (c *Controller) resolveScenePreviewAssetDirectory(rel string) string {
	if c == nil || c.assets == nil {
		return ""
	}
	for _, root := range c.assets.Roots() {
		if strings.HasPrefix(root, "http://") || strings.HasPrefix(root, "https://") {
			continue
		}
		candidate := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func scenePreviewAssetCandidates(region string, rel string) []string {
	clean := filepath.ToSlash(strings.TrimPrefix(rel, "/"))
	modes := []string{assets.RegionAssetOnDemand, assets.RegionAssetStartApp}
	if strings.HasPrefix(clean, "thumbnail/") || strings.HasPrefix(clean, "character/") {
		modes = []string{assets.RegionAssetStartApp, assets.RegionAssetOnDemand}
	}

	out := make([]string, 0, len(modes)*2+1)
	for _, mode := range modes {
		out = append(out, filepath.ToSlash(filepath.Join(assets.CloudRegionAssetDirByMode(region, mode), clean)))
		out = append(out, filepath.ToSlash(filepath.Join(assets.RegionAssetDirByMode(region, mode), clean)))
	}
	out = append(out, clean)
	return out
}

func captureScenePreview(ctx context.Context, targetURL string, width int, height int) ([]byte, error) {
	browserPath, err := findScenePreviewBrowser()
	if err != nil {
		return nil, err
	}

	timeout := scenePreviewTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browserPath),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("ignore-gpu-blocklist", true),
		chromedp.Flag("enable-webgl", true),
		chromedp.WindowSize(width, height),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	var screenshot []byte
	if err := chromedp.Run(browserCtx,
		emulation.SetDeviceMetricsOverride(int64(width), int64(height), 1, false),
		chromedp.Navigate(targetURL),
		waitScenePreviewReady(timeout),
		chromedp.CaptureScreenshot(&screenshot),
	); err != nil {
		return nil, fmt.Errorf("render mysekai scene preview: %w", err)
	}
	if len(screenshot) == 0 {
		return nil, fmt.Errorf("render mysekai scene preview produced empty screenshot")
	}
	return screenshot, nil
}

func waitScenePreviewReady(timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(timeout)
		var lastErr string
		for {
			var state struct {
				Ready bool   `json:"ready"`
				Error string `json:"error"`
			}
			err := chromedp.Evaluate(`({
				ready: window.__MYSEKAI_PREVIEW_READY === true || document.documentElement.dataset.mysekaiPreviewReady === "true",
				error: String(window.__MYSEKAI_PREVIEW_ERROR || document.documentElement.dataset.mysekaiPreviewError || "")
			})`, &state).Do(ctx)
			if err == nil {
				if state.Error != "" {
					return fmt.Errorf("mysekai scene preview page error: %s", state.Error)
				}
				if state.Ready {
					return nil
				}
			}
			if err != nil {
				lastErr = err.Error()
			}
			if time.Now().After(deadline) {
				if lastErr != "" {
					return fmt.Errorf("wait mysekai scene preview ready: %s", lastErr)
				}
				return fmt.Errorf("wait mysekai scene preview ready timed out")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	})
}

func findScenePreviewBrowser() (string, error) {
	if path := strings.TrimSpace(os.Getenv("HARUKI_MYSEKAI_PREVIEW_BROWSER_PATH")); path != "" {
		return path, nil
	}
	candidates := []string{
		"chromium",
		"chromium-browser",
		"google-chrome",
		"google-chrome-stable",
		"chrome",
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("mysekai scene preview browser is not available; install chromium or set HARUKI_MYSEKAI_PREVIEW_BROWSER_PATH")
}

func scenePreviewTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("HARUKI_MYSEKAI_PREVIEW_TIMEOUT"))
	if raw == "" {
		return scenePreviewDefaultTimeout
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return scenePreviewDefaultTimeout
	}
	return timeout
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}
