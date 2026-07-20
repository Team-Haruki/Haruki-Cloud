package mysekai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderassets "haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/utils/logger"

	"golang.org/x/sync/singleflight"
)

var (
	sekai8823FixtureFriendcodeURL = "https://pjsk-static.8823.eu.org/api/fixtures/"
	sekai8823FixtureFriendcodeTTL = 3 * time.Hour
	sekai8823FixtureHTTPClient    = &http.Client{Timeout: 10 * time.Second}

	fixtureFriendcodeCacheMu        sync.Mutex
	fixtureFriendcodeCacheFetchedAt time.Time
	fixtureFriendcodeCacheByID      map[int][]string
	fixtureFriendcodeCacheFlight    singleflight.Group
	mysekaiRunFixtureCodeCache      sync.Map

	fixtureFriendcodeRowRE = regexp.MustCompile(`(?is)<tr[^>]*>.*?</tr>`)
	fixtureFriendcodeTdRE  = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	fixtureFriendcodeTagRE = regexp.MustCompile(`(?is)<[^>]+>`)
)

const fixtureFriendcodeMaxResponseBytes int64 = 2 << 20

var (
	errFixtureFriendcodeResponseTooLarge = errors.New("fixture friendcodes: response exceeds size limit")
	errFixtureFriendcodeInvalidResponse  = errors.New("fixture friendcodes: invalid JSON response")
)

type fixtureFriendcodePayload struct {
	Fixtures []struct {
		ID          int      `json:"id"`
		FriendCodes []string `json:"friendCodes"`
	} `json:"fixtures"`
}

type fixtureFriendcodeFlightResult struct {
	cache      map[int][]string
	operations []commandtrace.Stats
	leader     *fixtureFriendcodeFlightToken
}

type fixtureFriendcodeFlightToken byte

type mysekaiRunFixtureCodeCacheEntry struct {
	signature string
	index     map[string][]string
}

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
	if c == nil || c.resolver == nil || !c.resolver.allowFallback {
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
	if codes := loadSekai8823FixtureFriendcodes(c.requestCtx, fixtureID); len(codes) > 0 {
		return codes, "sekai.8823.eu.org"
	}
	if codes := c.loadMySekaiRunFixtureFriendcodes(region, fixtureName); len(codes) > 0 {
		return codes, "my.sekai.run"
	}
	return nil, ""
}

func loadSekai8823FixtureFriendcodes(ctx context.Context, fixtureID int) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	finishLookup := commandtrace.MeasureOperation(ctx, "fixture_friendcodes.cache_lookup")
	fixtureFriendcodeCacheMu.Lock()
	now := time.Now()
	if fixtureFriendcodeCacheByID != nil && now.Before(fixtureFriendcodeCacheFetchedAt.Add(sekai8823FixtureFriendcodeTTL)) {
		codes := limitFixtureFriendcodes(fixtureFriendcodeCacheByID[fixtureID])
		fixtureFriendcodeCacheMu.Unlock()
		finishLookup()
		return codes
	}
	fixtureFriendcodeCacheMu.Unlock()
	finishLookup()

	finishWait := commandtrace.MeasureOperation(ctx, "fixture_friendcodes.wait")
	callerToken := new(fixtureFriendcodeFlightToken)
	resultCh := fixtureFriendcodeCacheFlight.DoChan("all", func() (any, error) {
		timeout := sekai8823FixtureHTTPClient.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		detached := context.Background()
		detached = logger.WithContextAttrs(detached, slog.Bool("shared_work", true))
		sharedBase, cancel := context.WithTimeout(detached, timeout)
		defer cancel()
		sharedCtx, trace := commandtrace.WithNewTrace(sharedBase)

		fixtureFriendcodeCacheMu.Lock()
		now := time.Now()
		if fixtureFriendcodeCacheByID != nil && now.Before(fixtureFriendcodeCacheFetchedAt.Add(sekai8823FixtureFriendcodeTTL)) {
			cache := fixtureFriendcodeCacheByID
			fixtureFriendcodeCacheMu.Unlock()
			return fixtureFriendcodeFlightResult{cache: cache, operations: trace.Snapshot().Operations, leader: callerToken}, nil
		}
		fixtureFriendcodeCacheMu.Unlock()

		req, err := http.NewRequestWithContext(sharedCtx, http.MethodGet, sekai8823FixtureFriendcodeURL, nil)
		if err != nil {
			return fixtureFriendcodeFlightResult{operations: trace.Snapshot().Operations, leader: callerToken}, nil
		}
		finishHTTP := commandtrace.MeasureOperation(sharedCtx, "fixture_friendcodes.http")
		resp, err := sekai8823FixtureHTTPClient.Do(req)
		finishHTTP()
		if err != nil {
			return fixtureFriendcodeFlightResult{operations: trace.Snapshot().Operations, leader: callerToken}, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fixtureFriendcodeFlightResult{operations: trace.Snapshot().Operations, leader: callerToken}, nil
		}

		finishDecode := commandtrace.MeasureOperation(sharedCtx, "fixture_friendcodes.decode")
		payload, decodeErr := decodeFixtureFriendcodePayload(resp.Body)
		finishDecode()
		if decodeErr != nil {
			return fixtureFriendcodeFlightResult{operations: trace.Snapshot().Operations, leader: callerToken}, nil
		}

		cache := make(map[int][]string, len(payload.Fixtures))
		for _, item := range payload.Fixtures {
			cache[item.ID] = normalizeFixtureFriendcodes(item.FriendCodes)
		}
		fixtureFriendcodeCacheMu.Lock()
		fixtureFriendcodeCacheByID = cache
		fixtureFriendcodeCacheFetchedAt = now
		fixtureFriendcodeCacheMu.Unlock()
		return fixtureFriendcodeFlightResult{cache: cache, operations: trace.Snapshot().Operations, leader: callerToken}, nil
	})
	select {
	case <-ctx.Done():
		finishWait()
		return nil
	case completed := <-resultCh:
		finishWait()
		result, ok := completed.Val.(fixtureFriendcodeFlightResult)
		if !ok {
			return nil
		}
		commandtrace.MergeOperations(ctx, result.operations)
		if result.leader != callerToken {
			commandtrace.RecordOperation(ctx, "fixture_friendcodes.shared", 0)
		}
		return limitFixtureFriendcodes(result.cache[fixtureID])
	}
}

func decodeFixtureFriendcodePayload(reader io.Reader) (fixtureFriendcodePayload, error) {
	limited := &io.LimitedReader{R: reader, N: fixtureFriendcodeMaxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	var payload fixtureFriendcodePayload
	if err := decoder.Decode(&payload); err != nil {
		if limited.N == 0 {
			return fixtureFriendcodePayload{}, errFixtureFriendcodeResponseTooLarge
		}
		return fixtureFriendcodePayload{}, errFixtureFriendcodeInvalidResponse
	}

	var trailing any
	err := decoder.Decode(&trailing)
	if limited.N == 0 {
		return fixtureFriendcodePayload{}, errFixtureFriendcodeResponseTooLarge
	}
	if !errors.Is(err, io.EOF) {
		return fixtureFriendcodePayload{}, errFixtureFriendcodeInvalidResponse
	}
	return payload, nil
}

func (c *Controller) loadMySekaiRunFixtureFriendcodes(region renderregion.Value, fixtureName string) []string {
	fixtureName = strings.TrimSpace(fixtureName)
	if fixtureName == "" {
		return nil
	}
	filename := filepath.Join("mysekairun", strings.ToLower(strings.TrimSpace(renderregion.WithDefault(region).String()))+".html")
	for _, dir := range c.localMasterdataDirs(region) {
		fullPath := filepath.Join(dir, filename)
		finishLookup := commandtrace.MeasureOperation(c.requestCtx, "fixture_friendcodes.local_lookup")
		info, statErr := os.Stat(fullPath)
		signature := ""
		if statErr == nil {
			signature = info.ModTime().UTC().Format(time.RFC3339Nano) + "|" + fmt.Sprint(info.Size())
			if cached, ok := mysekaiRunFixtureCodeCache.Load(fullPath); ok {
				entry, _ := cached.(mysekaiRunFixtureCodeCacheEntry)
				if entry.signature == signature && entry.index != nil {
					finishLookup()
					return limitFixtureFriendcodes(entry.index[fixtureName])
				}
			}
		}
		finishLookup()
		finishRead := commandtrace.MeasureOperation(c.requestCtx, "fixture_friendcodes.local_read")
		data, err := os.ReadFile(fullPath)
		finishRead()
		if err != nil {
			continue
		}
		finishParse := commandtrace.MeasureOperation(c.requestCtx, "fixture_friendcodes.local_parse")
		index := indexFixtureFriendcodesFromHTML(string(data))
		finishParse()
		if signature != "" {
			mysekaiRunFixtureCodeCache.Store(fullPath, mysekaiRunFixtureCodeCacheEntry{
				signature: signature,
				index:     index,
			})
		}
		return limitFixtureFriendcodes(index[fixtureName])
	}
	return nil
}

func indexFixtureFriendcodesFromHTML(content string) map[string][]string {
	result := make(map[string][]string)
	rows := fixtureFriendcodeRowRE.FindAllString(content, -1)
	for _, row := range rows {
		cols := fixtureFriendcodeTdRE.FindAllStringSubmatch(row, -1)
		if len(cols) < 3 {
			continue
		}
		name := cleanFixtureFriendcodeHTML(cols[1][1])
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cell := cols[2][1]
		cell = strings.ReplaceAll(cell, "<br/>", "\n")
		cell = strings.ReplaceAll(cell, "<br />", "\n")
		cell = strings.ReplaceAll(cell, "<br>", "\n")
		raw := cleanFixtureFriendcodeHTML(cell)
		if _, exists := result[name]; !exists {
			result[name] = normalizeFixtureFriendcodes(strings.Fields(raw))
		}
	}
	return result
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
