package music

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type customChartEntry struct {
	ID                  string
	Title               string
	Path                string
	MusicID             int
	Difficulty          string
	PlayLevel           int
	UserName            string
	Description         string
	PreviewStartTimeSec float64
	PublishedAt         int64
	ReviewCount         int
	PlayCount           int
	FullComboRate       float64
}

type customChartStats struct {
	NoteCount int
	BPM       string
}

var customChartPrefixes = map[string]struct{}{
	"custom":      {},
	"customscore": {},
	"customchart": {},
	"自制谱":         {},
	"自制谱面":        {},
	"自定义谱":        {},
	"自定义谱面":       {},
	"自定谱":         {},
	"自定谱面":        {},
}

func IsCustomChartQuery(query string) bool {
	_, ok := stripCustomChartPrefix(query)
	return ok
}

func IsCustomChartIDQuery(query string) bool {
	_, ok := customChartIDFromQuery(query)
	return ok
}

func stripCustomChartPrefix(query string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return "", false
	}
	prefix := strings.ToLower(strings.TrimSpace(fields[0]))
	if _, ok := customChartPrefixes[prefix]; !ok {
		return strings.TrimSpace(query), false
	}
	return strings.TrimSpace(strings.Join(fields[1:], " ")), true
}

func (c *Controller) buildCustomMusicChartRequest(query ChartQuery, source DataSource, builder *Builder, region renderregion.Value) (*drawing.GenerateMusicChartRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if c.customScores == nil {
		return nil, fmt.Errorf("自制谱面数据源未配置")
	}
	if region != renderregion.JP {
		return nil, fmt.Errorf("当前服务器暂未支持自定义谱面请使用jp前缀查询")
	}

	keyword := strings.TrimSpace(query.Query)
	if stripped, ok := stripCustomChartPrefix(keyword); ok {
		keyword = stripped
	}
	scoreID, ok := customChartIDFromCleanKeyword(keyword)
	if !ok {
		return nil, fmt.Errorf("请提供28位自定义谱面ID")
	}

	entry, err := c.fetchCustomChartEntryByID(region.String(), scoreID)
	if err != nil {
		return nil, err
	}

	rawScore, err := c.customScores.GetCustomMusicScore(region.String(), entry.Path)
	if err != nil {
		return nil, fmt.Errorf("获取自制谱面 JSON 失败: %w", err)
	}
	chartJSON, err := decodeCustomMusicScoreJSON(rawScore)
	if err != nil {
		return nil, err
	}

	musicInfo, err := source.GetMusicByID(entry.MusicID)
	if err != nil || musicInfo == nil {
		return nil, fmt.Errorf("自制谱面对应的原曲数据不存在")
	}

	diff := strings.ToLower(strings.TrimSpace(entry.Difficulty))
	if diff == "" {
		diff = normalizeDifficulty(query.Difficulty)
	}
	if diff == "" {
		diff = "master"
	}

	playLevel := any(entry.PlayLevel)
	if entry.PlayLevel <= 0 {
		playLevel = "?"
	}

	title := buildCustomChartTitle(builder.buildDisplayMusicTitle(musicInfo, region), entry.Title)

	artist := buildCustomChartArtist(entry)

	jacketPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
	stylePath := chartstyle.CSSPath(query.Style)
	assetBase := builder.assets.Primary()
	req := &drawing.GenerateMusicChartRequest{
		MusicID:              customChartCacheID(entry),
		Title:                title,
		Artist:               artist,
		Difficulty:           diff,
		PlayLevel:            playLevel,
		Skill:                query.Skill,
		JacketPath:           assets.MakeRelative(assetBase, jacketPath),
		ChartJSON:            &chartJSON,
		StylePath:            &stylePath,
		NoteHost:             assets.StaticImagesDir + "/chart_asset/notes",
		TargetSegmentSeconds: float64Ptr(6.0),
	}
	if query.Skill {
		req.MusicMeta = c.resolveMusicChartMeta(region, musicInfo.ID, diff)
	}
	return req, nil
}

func (c *Controller) buildCustomMusicDetailRequest(query Query, source DataSource, builder *Builder, region renderregion.Value) (*drawing.MusicDetailRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if c.customScores == nil {
		return nil, fmt.Errorf("自制谱面数据源未配置")
	}
	if region != renderregion.JP {
		return nil, fmt.Errorf("当前服务器暂未支持自定义谱面请使用jp前缀查询")
	}

	scoreID, ok := customChartIDFromQuery(query.Query)
	if !ok {
		return nil, fmt.Errorf("请提供28位自定义谱面ID")
	}
	entry, err := c.fetchCustomChartEntryByID(region.String(), scoreID)
	if err != nil {
		return nil, err
	}

	rawScore, err := c.customScores.GetCustomMusicScore(region.String(), entry.Path)
	if err != nil {
		return nil, fmt.Errorf("获取自制谱面 JSON 失败: %w", err)
	}
	chartJSON, err := decodeCustomMusicScoreJSON(rawScore)
	if err != nil {
		return nil, err
	}
	stats := parseCustomMusicScoreStats(chartJSON)

	musicInfo, err := source.GetMusicByID(entry.MusicID)
	if err != nil || musicInfo == nil {
		return nil, fmt.Errorf("自制谱面对应的原曲数据不存在")
	}

	req, err := builder.BuildMusicDetailRequest(musicInfo, region)
	if err != nil {
		return nil, err
	}
	c.enrichMusicDetailRequest(req, region, source, builder, musicInfo, entry.Difficulty)

	diff := strings.ToLower(strings.TrimSpace(entry.Difficulty))
	if diff == "" {
		diff = "master"
	}
	req.Difficulty = drawing.DifficultyInfo{
		Level:     []int{entry.PlayLevel},
		NoteCount: []int{stats.NoteCount},
		HasAppend: diff == "append",
		Order:     []string{diff},
	}
	req.Alias = nil
	req.LeaderboardMatrix = nil
	req.LeaderboardMusicNum = nil
	req.LeaderboardLiveTypes = nil
	req.LeaderboardTargets = nil
	req.CustomChartInfo = &drawing.CustomChartInfo{
		ScoreID:             entry.ID,
		Title:               entry.Title,
		Author:              entry.UserName,
		Description:         entry.Description,
		Difficulty:          diff,
		PlayLevel:           entry.PlayLevel,
		NoteCount:           stats.NoteCount,
		BPM:                 stats.BPM,
		PublishedAt:         entry.PublishedAt,
		PreviewStartTimeSec: entry.PreviewStartTimeSec,
		ReviewCount:         entry.ReviewCount,
		PlayCount:           entry.PlayCount,
		FullComboRate:       entry.FullComboRate,
		Tags:                req.MusicInfo.Categories,
	}
	return req, nil
}

func buildCustomChartArtist(entry customChartEntry) string {
	userName := strings.TrimSpace(entry.UserName)
	scoreID := strings.TrimSpace(entry.ID)
	switch {
	case userName != "" && scoreID != "":
		return userName + "/" + scoreID
	case userName != "":
		return userName
	case scoreID != "":
		return scoreID
	default:
		return "自制谱"
	}
}

func buildCustomChartTitle(originalTitle string, customTitle string) string {
	originalTitle = strings.TrimSpace(originalTitle)
	customTitle = strings.TrimSpace(customTitle)
	switch {
	case originalTitle != "" && customTitle != "" && customTitle != originalTitle:
		return originalTitle + "/" + customTitle
	case customTitle != "":
		return customTitle
	default:
		return originalTitle
	}
}

func (c *Controller) fetchCustomChartEntryByID(region string, scoreID string) (customChartEntry, error) {
	published, err := c.customScores.GetCustomMusicScorePublished(region, scoreID)
	if err != nil {
		if isCustomChartNotFoundError(err) {
			return customChartEntry{}, fmt.Errorf("未找到对应自定义谱面")
		}
		return customChartEntry{}, fmt.Errorf("获取自定义谱面信息失败: %w", err)
	}
	entry := customChartEntryFromPublishedResponse(published)
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Path) == "" {
		return customChartEntry{}, fmt.Errorf("未找到对应自定义谱面")
	}
	return entry, nil
}

func isCustomChartNotFoundError(err error) bool {
	if errors.Is(err, sekaiapi.ErrUserNotFound) {
		return true
	}
	var apiErr *sekaiapi.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 404 {
			return true
		}
		message := strings.ToLower(apiErr.Message)
		return strings.Contains(message, "status=404") || strings.Contains(message, "status 404") || strings.Contains(message, "not found")
	}
	return false
}

func customChartEntryFromPublishedResponse(value *sekaiapi.UserCustomMusicScorePublishedResponse) customChartEntry {
	if value == nil {
		return customChartEntry{}
	}
	info := value.UserCustomMusicScoreInfoJSON
	entry := customChartEntry{
		ID:                  strings.TrimSpace(value.UserCustomMusicScoreID),
		MusicID:             value.MusicID,
		Difficulty:          strings.TrimSpace(value.MusicDifficultyType),
		PlayLevel:           value.PlayLevel,
		UserName:            strings.TrimSpace(value.UserName),
		Description:         strings.TrimSpace(value.Description),
		PreviewStartTimeSec: value.PreviewStartTimeSec,
		PublishedAt:         value.PublishedAt,
		ReviewCount:         value.ReviewCount,
		PlayCount:           value.PlayCount,
		FullComboRate:       value.FullComboRate,
	}
	if info != nil {
		entry.Title = strings.TrimSpace(info.Title)
		entry.Path = strings.TrimSpace(info.UserCustomMusicScorePath)
		if entry.MusicID == 0 {
			entry.MusicID = info.MusicID
		}
	}
	return entry
}

func parseCustomMusicScoreStats(chartJSON string) customChartStats {
	raw := []byte(strings.TrimSpace(chartJSON))
	if len(raw) == 0 {
		return customChartStats{}
	}

	var root map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(raw, &root); err != nil {
		return customChartStats{}
	}
	chartRaw := raw
	if nested, ok := root["chart"]; ok && len(nested) > 0 {
		chartRaw = nested
	}

	var chart struct {
		MusicScoreEventDataList []struct {
			EventType   int `json:"eventType"`
			ChangeValue any `json:"changeValue"`
		} `json:"MusicScoreEventDataList"`
		NoteList []stdjson.RawMessage `json:"NoteList"`
	}
	if err := stdjson.Unmarshal(chartRaw, &chart); err != nil {
		return customChartStats{}
	}

	return customChartStats{
		NoteCount: len(chart.NoteList),
		BPM:       formatCustomChartBPMs(chart.MusicScoreEventDataList),
	}
}

func formatCustomChartBPMs(events []struct {
	EventType   int `json:"eventType"`
	ChangeValue any `json:"changeValue"`
}) string {
	values := make([]float64, 0, len(events))
	seen := make(map[string]struct{})
	for _, event := range events {
		if event.EventType != 0 {
			continue
		}
		bpm, ok := customChartFloatValue(event.ChangeValue)
		if !ok || bpm <= 0 {
			continue
		}
		key := formatCustomChartFloat(bpm)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, bpm)
	}
	if len(values) == 0 {
		return ""
	}
	if len(values) <= 3 {
		labels := make([]string, 0, len(values))
		for _, bpm := range values {
			labels = append(labels, formatCustomChartFloat(bpm))
		}
		return strings.Join(labels, " / ")
	}
	sortedValues := append([]float64(nil), values...)
	sort.Float64s(sortedValues)
	return fmt.Sprintf("%s-%s（%d段）", formatCustomChartFloat(sortedValues[0]), formatCustomChartFloat(sortedValues[len(sortedValues)-1]), len(values))
}

func customChartFloatValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case stdjson.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func formatCustomChartFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

func decodeCustomMusicScoreJSON(raw []byte) (string, error) {
	decoded, err := decodeCustomMusicScoreJSONBytes(bytes.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func decodeCustomMusicScoreJSONBytes(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("自制谱面 JSON 为空")
	}

	if jsonFromEnvelope, ok, err := decodeCustomMusicScoreEnvelope(raw); ok || err != nil {
		return jsonFromEnvelope, err
	}
	if unzipped, ok, err := gunzipMaybe(raw); err != nil {
		return nil, err
	} else if ok {
		return ensureJSONBytes(unzipped)
	}
	if decoded, ok, err := base64DecodeMaybe(string(raw)); err != nil {
		return nil, err
	} else if ok {
		if unzipped, zipped, zipErr := gunzipMaybe(decoded); zipped || zipErr != nil {
			if zipErr != nil {
				return nil, zipErr
			}
			return ensureJSONBytes(unzipped)
		}
		return ensureJSONBytes(decoded)
	}
	return ensureJSONBytes(raw)
}

func decodeCustomMusicScoreEnvelope(raw []byte) ([]byte, bool, error) {
	if !looksLikeJSON(raw) {
		return nil, false, nil
	}
	var obj map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(raw, &obj); err != nil {
		return nil, true, fmt.Errorf("解析自制谱面 JSON 失败: %w", err)
	}
	for _, key := range []string{"userCustomMusicScoreJsonGzipBase64", "userCustomMusicScorePreviewJsonGzipBase64"} {
		value, ok := obj[key]
		if !ok || len(value) == 0 {
			continue
		}
		var encoded string
		if err := stdjson.Unmarshal(value, &encoded); err != nil {
			continue
		}
		decoded, ok, err := base64DecodeMaybe(encoded)
		if err != nil || !ok {
			return nil, true, err
		}
		unzipped, zipped, err := gunzipMaybe(decoded)
		if err != nil {
			return nil, true, err
		}
		if zipped {
			out, err := ensureJSONBytes(unzipped)
			return out, true, err
		}
		out, err := ensureJSONBytes(decoded)
		return out, true, err
	}
	return raw, true, nil
}

func gunzipMaybe(raw []byte) ([]byte, bool, error) {
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return nil, false, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, true, fmt.Errorf("解压自制谱面 JSON 失败: %w", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, true, fmt.Errorf("读取自制谱面 JSON 失败: %w", err)
	}
	return decoded, true, nil
}

func base64DecodeMaybe(value string) ([]byte, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false, nil
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, true, nil
		}
	}
	return nil, false, nil
}

func ensureJSONBytes(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if looksLikeJSON(raw) && stdjson.Valid(raw) {
		return raw, nil
	}
	return nil, fmt.Errorf("自制谱面 JSON 格式无效")
}

func looksLikeJSON(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && (raw[0] == '{' || raw[0] == '[')
}

func customChartCacheID(entry customChartEntry) string {
	if id := strings.TrimSpace(entry.ID); id != "" {
		return id
	}
	return "custom_unknown"
}

func customChartIDFromQuery(query string) (string, bool) {
	query = strings.TrimSpace(query)
	if stripped, ok := stripCustomChartPrefix(query); ok {
		query = stripped
	}
	if scoreID, ok := customChartIDFromCleanKeyword(query); ok {
		return scoreID, true
	}
	return "", false
}

func customChartIDFromCleanKeyword(keyword string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(keyword))
	if len(fields) != 1 {
		return "", false
	}
	if len([]rune(fields[0])) != 28 {
		return "", false
	}
	return fields[0], true
}
