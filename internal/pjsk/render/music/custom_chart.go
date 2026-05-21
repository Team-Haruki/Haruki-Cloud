package music

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"haruki-cloud/internal/pjsk/chartstyle"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

type customChartEntry struct {
	ID         string
	Title      string
	Path       string
	MusicID    int
	Difficulty string
	PlayLevel  int
	UserName   string
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
		if errors.Is(err, sekaiapi.ErrUserNotFound) {
			return customChartEntry{}, fmt.Errorf("未查找到该自定义谱面")
		}
		return customChartEntry{}, fmt.Errorf("获取自定义谱面信息失败: %w", err)
	}
	entry := customChartEntryFromPublishedResponse(published)
	if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Path) == "" {
		return customChartEntry{}, fmt.Errorf("未查找到该自定义谱面")
	}
	return entry, nil
}

func customChartEntryFromPublishedResponse(value *sekaiapi.UserCustomMusicScorePublishedResponse) customChartEntry {
	if value == nil {
		return customChartEntry{}
	}
	info := value.UserCustomMusicScoreInfoJSON
	entry := customChartEntry{
		ID:         strings.TrimSpace(value.UserCustomMusicScoreID),
		MusicID:    value.MusicID,
		Difficulty: strings.TrimSpace(value.MusicDifficultyType),
		PlayLevel:  value.PlayLevel,
		UserName:   strings.TrimSpace(value.UserName),
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
