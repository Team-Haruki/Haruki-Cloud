package mysekai

import (
	"encoding/json"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
)

var (
	sekai8823FixtureFriendcodeURL = "https://pjsk-static.8823.eu.org/api/fixtures/"
	sekai8823FixtureFriendcodeTTL = 3 * time.Hour
	sekai8823FixtureHTTPClient    = &http.Client{Timeout: 10 * time.Second}

	fixtureFriendcodeCacheMu        sync.Mutex
	fixtureFriendcodeCacheFetchedAt time.Time
	fixtureFriendcodeCacheByID      map[int][]string

	fixtureFriendcodeRowRE = regexp.MustCompile(`(?is)<tr[^>]*>.*?</tr>`)
	fixtureFriendcodeTdRE  = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	fixtureFriendcodeTagRE = regexp.MustCompile(`(?is)<[^>]+>`)
)

func (c *Controller) loadFixtureReactionObject(region renderregion.Value, target any) bool {
	paths := []string{
		"mysekai/system/fixture_reaction_data/fixture_reaction_data.json",
		"mysekai/system/fixture_reaction_data_rip/fixture_reaction_data.asset",
	}
	for _, path := range paths {
		if c.masterdata != nil && c.masterdata.loadObject(path, target) {
			return true
		}
		if c.loadLocalMasterdataObject(region, path, target) {
			return true
		}
		if c.loadLocalAssetObject(region, path, target) {
			return true
		}
	}
	return false
}

func (c *Controller) loadLocalMasterdataObject(region renderregion.Value, filename string, target any) bool {
	for _, dir := range c.localMasterdataDirs(region) {
		data, err := os.ReadFile(filepath.Join(dir, filepath.Clean(filename)))
		if err != nil {
			continue
		}
		return decodeJSONUseNumber(data, target) == nil
	}
	return false
}

func (c *Controller) loadLocalAssetObject(region renderregion.Value, relPath string, target any) bool {
	if c == nil || c.assets == nil {
		return false
	}
	regionName := strings.ToLower(strings.TrimSpace(renderregion.WithDefault(region).String()))
	if regionName == "" {
		return false
	}
	candidates := []string{
		filepath.ToSlash(filepath.Join(renderassets.RegionAssetDirByMode(regionName, renderassets.RegionAssetOnDemand), relPath)),
		filepath.ToSlash(filepath.Join(regionName+"-assets", renderassets.RegionAssetOnDemand, relPath)),
	}
	for _, candidate := range candidates {
		resolved := c.assets.FirstExisting(candidate)
		if strings.TrimSpace(resolved) == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(resolved))
		if err != nil {
			continue
		}
		return decodeJSONUseNumber(data, target) == nil
	}
	return false
}

func (c *Controller) localMasterdataDirs(region renderregion.Value) []string {
	if c == nil || c.resolver == nil {
		return nil
	}
	base := strings.TrimSpace(c.resolver.localDir)
	if base == "" {
		return nil
	}
	base = filepath.Clean(base)
	seen := make(map[string]struct{}, 2)
	dirs := make([]string, 0, 2)
	if resolved := renderregion.WithDefault(region).String(); strings.TrimSpace(resolved) != "" {
		regionDir := filepath.Join(base, resolved)
		if _, ok := seen[regionDir]; !ok {
			seen[regionDir] = struct{}{}
			dirs = append(dirs, regionDir)
		}
	}
	if _, ok := seen[base]; !ok {
		dirs = append(dirs, base)
	}
	return dirs
}

func (c *Controller) fixtureFriendcodes(region renderregion.Value, fixtureID int, fixtureName string, sketchable bool) ([]string, string) {
	if !sketchable || fixtureID <= 0 || !strings.EqualFold(renderregion.WithDefault(region).String(), renderregion.JP.String()) {
		return nil, ""
	}
	if codes := loadSekai8823FixtureFriendcodes(fixtureID); len(codes) > 0 {
		return codes, "sekai.8823.eu.org"
	}
	if codes := c.loadMySekaiRunFixtureFriendcodes(region, fixtureName); len(codes) > 0 {
		return codes, "my.sekai.run"
	}
	return nil, ""
}

func loadSekai8823FixtureFriendcodes(fixtureID int) []string {
	fixtureFriendcodeCacheMu.Lock()
	defer fixtureFriendcodeCacheMu.Unlock()

	now := time.Now()
	if fixtureFriendcodeCacheByID != nil && now.Before(fixtureFriendcodeCacheFetchedAt.Add(sekai8823FixtureFriendcodeTTL)) {
		return limitFixtureFriendcodes(fixtureFriendcodeCacheByID[fixtureID])
	}

	req, err := http.NewRequest(http.MethodGet, sekai8823FixtureFriendcodeURL, nil)
	if err != nil {
		return nil
	}
	resp, err := sekai8823FixtureHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var payload struct {
		Fixtures []struct {
			ID          int      `json:"id"`
			FriendCodes []string `json:"friendCodes"`
		} `json:"fixtures"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}

	cache := make(map[int][]string, len(payload.Fixtures))
	for _, item := range payload.Fixtures {
		cache[item.ID] = normalizeFixtureFriendcodes(item.FriendCodes)
	}
	fixtureFriendcodeCacheByID = cache
	fixtureFriendcodeCacheFetchedAt = now
	return limitFixtureFriendcodes(cache[fixtureID])
}

func (c *Controller) loadMySekaiRunFixtureFriendcodes(region renderregion.Value, fixtureName string) []string {
	fixtureName = strings.TrimSpace(fixtureName)
	if fixtureName == "" {
		return nil
	}
	filename := filepath.Join("mysekairun", strings.ToLower(strings.TrimSpace(renderregion.WithDefault(region).String()))+".html")
	for _, dir := range c.localMasterdataDirs(region) {
		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			continue
		}
		return limitFixtureFriendcodes(extractFixtureFriendcodesFromHTML(string(data), fixtureName))
	}
	return nil
}

func extractFixtureFriendcodesFromHTML(content string, fixtureName string) []string {
	rows := fixtureFriendcodeRowRE.FindAllString(content, -1)
	for _, row := range rows {
		cols := fixtureFriendcodeTdRE.FindAllStringSubmatch(row, -1)
		if len(cols) < 3 {
			continue
		}
		name := cleanFixtureFriendcodeHTML(cols[1][1])
		if strings.TrimSpace(name) != fixtureName {
			continue
		}
		cell := cols[2][1]
		cell = strings.ReplaceAll(cell, "<br/>", "\n")
		cell = strings.ReplaceAll(cell, "<br />", "\n")
		cell = strings.ReplaceAll(cell, "<br>", "\n")
		raw := cleanFixtureFriendcodeHTML(cell)
		return normalizeFixtureFriendcodes(strings.Fields(raw))
	}
	return nil
}

func cleanFixtureFriendcodeHTML(raw string) string {
	clean := fixtureFriendcodeTagRE.ReplaceAllString(raw, " ")
	clean = html.UnescapeString(clean)
	return strings.Join(strings.Fields(clean), " ")
}

func normalizeFixtureFriendcodes(codes []string) []string {
	seen := make(map[string]struct{}, len(codes))
	result := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func limitFixtureFriendcodes(codes []string) []string {
	codes = normalizeFixtureFriendcodes(codes)
	if len(codes) > 4 {
		return append([]string(nil), codes[:4]...)
	}
	return append([]string(nil), codes...)
}
