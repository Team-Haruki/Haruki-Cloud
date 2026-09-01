package music

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (c *Controller) ResolveMusicCover(query Query) (*CoverResult, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := c.newSearchService(source, c.shouldAllowLookupLeaks(query.Region, query.AllowUnreleased))
	musicInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}

	jacketPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
	if localPath := c.resolveLocalMusicJacket(musicInfo.AssetBundleName); localPath != "" {
		jacketPath = localPath
	}
	if strings.TrimSpace(jacketPath) == "" {
		return nil, fmt.Errorf("music %d does not have jacket asset", musicInfo.ID)
	}

	return &CoverResult{
		Music:      buildLookupMusic(musicInfo, builder, region),
		JacketPath: jacketPath,
	}, nil
}

func (c *Controller) FindMusicChartsByBPM(query BPMQuery) ([]BPMMatch, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	ctx := c.contextOrBackground()
	finishLookup := commandtrace.MeasureOperation(ctx, "music.bpm_lookup")
	defer finishLookup()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query.BPM <= 0 {
		return nil, fmt.Errorf("BPM 必须大于 0")
	}

	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}

	finishScan := commandtrace.MeasureOperation(ctx, "music.chart_scan")
	matches, err := c.scanMusicChartsByBPM(ctx, source, builder, region, query)
	finishScan()
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("没有找到 BPM 为 %s 的谱面", formatLookupBPMValue(query.BPM))
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Music.ID == matches[j].Music.ID {
			return difficultyOrder(matches[i].Difficulty) < difficultyOrder(matches[j].Difficulty)
		}
		return matches[i].Music.ID < matches[j].Music.ID
	})
	return matches, nil
}

func (c *Controller) scanMusicChartsByBPM(ctx context.Context, source DataSource, builder *Builder, region renderregion.Value, query BPMQuery) ([]BPMMatch, error) {
	now := currentMusicVisibilityTime()
	matches := make([]BPMMatch, 0)
	for _, musicInfo := range source.GetMusics() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isMusicVisibleAt(musicInfo, now) {
			continue
		}
		musicMatches, err := c.scanOneMusicChartsByBPM(ctx, source, builder, region, musicInfo, query)
		if err != nil {
			return nil, err
		}
		matches = append(matches, musicMatches...)
	}
	return matches, nil
}

func (c *Controller) scanOneMusicChartsByBPM(ctx context.Context, source DataSource, builder *Builder, region renderregion.Value, musicInfo *masterdata.Music, query BPMQuery) ([]BPMMatch, error) {
	matches := make([]BPMMatch, 0)
	for _, difficulty := range c.collectBPMSearchDifficulties(source, musicInfo.ID, query.Difficulty) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chartPath := c.resolveLocalChartPath(region.String(), musicInfo.ID, difficulty)
		if chartPath == "" {
			continue
		}
		parsed, err := parseChartBPM(ctx, chartPath)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if chartContainsBPM(parsed, query.BPM) {
			matches = append(matches, BPMMatch{
				Music: buildLookupMusic(musicInfo, builder, region), Difficulty: difficulty,
				MainBPM: parsed.MainBPM, Events: parsed.Events,
			})
		}
	}
	return matches, nil
}

func (c *Controller) ResolveMusicBPM(query Query) (*BPMResult, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	ctx := c.contextOrBackground()
	finishLookup := commandtrace.MeasureOperation(ctx, "music.bpm_lookup")
	defer finishLookup()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parser := NewParser(c.banCharacterNicknames)
	if preferred, cleaned := parser.extractDiff(query.Query); preferred != "" && strings.TrimSpace(query.Difficulty) == "" {
		query.Difficulty = preferred
		query.Query = cleaned
	}

	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := c.newSearchService(source, false)
	searcher.parser = parser
	musicInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}

	difficulties := buildBPMDifficultyCandidates(query.Difficulty)
	var (
		chartPath string
		diffUsed  string
	)
	finishScan := commandtrace.MeasureOperation(ctx, "music.chart_scan")
	for _, difficulty := range difficulties {
		if err := ctx.Err(); err != nil {
			finishScan()
			return nil, err
		}
		chartPath = c.resolveLocalChartPath(region.String(), musicInfo.ID, difficulty)
		if chartPath == "" {
			continue
		}
		diffUsed = difficulty
		break
	}
	if chartPath == "" {
		finishScan()
		return nil, fmt.Errorf("当前环境没有可读取的本地谱面文件，无法查询 BPM")
	}

	parsed, err := parseChartBPM(ctx, chartPath)
	finishScan()
	if err != nil {
		return nil, err
	}

	jacketPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName, region)
	if localPath := c.resolveLocalMusicJacket(musicInfo.AssetBundleName); localPath != "" {
		jacketPath = localPath
	}

	return &BPMResult{
		Music:      buildLookupMusic(musicInfo, builder, region),
		JacketPath: jacketPath,
		Difficulty: diffUsed,
		MainBPM:    parsed.MainBPM,
		Events:     parsed.Events,
		BarCount:   parsed.BarCount,
		Duration:   parsed.Duration,
	}, nil
}

func (c *Controller) resolveLocalMusicJacket(assetName string) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetName) == "" {
		return ""
	}
	return c.assets.FirstExisting(
		filepath.Join("music", "jacket", assetName, assetName+".png"),
	)
}

func (c *Controller) resolveLocalChartPath(region string, musicID int, difficulty string) string {
	if c == nil || c.assets == nil || musicID <= 0 || strings.TrimSpace(difficulty) == "" {
		return ""
	}
	diff := normalizeDifficulty(difficulty)
	if strings.TrimSpace(region) == "" {
		region = "jp"
	}

	relPaths := []string{
		filepath.Join("music", "music_score", fmt.Sprintf("%04d_01", musicID), diff+".txt"),
	}
	candidates := make([]string, 0, len(relPaths)*3)
	for _, relPath := range relPaths {
		candidates = append(candidates,
			relPath,
			filepath.Join(assets.CloudRegionAssetDirByMode(region, assets.RegionAssetStartApp), relPath),
			filepath.Join(assets.RegionAssetDirByMode(region, assets.RegionAssetStartApp), relPath),
		)
	}
	return c.assets.FirstExisting(candidates...)
}

func (c *Controller) collectBPMSearchDifficulties(source DataSource, musicID int, preferred string) []string {
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		return []string{normalizeDifficulty(preferred)}
	}

	difficulties, err := source.GetMusicDifficulties(musicID)
	if err != nil || len(difficulties) == 0 {
		return buildBPMDifficultyCandidates("")
	}

	seen := make(map[string]struct{}, len(difficulties))
	result := make([]string, 0, len(difficulties))
	for _, difficulty := range difficulties {
		if difficulty == nil {
			continue
		}
		diff := normalizeDifficulty(difficulty.MusicDifficulty)
		if _, ok := seen[diff]; ok {
			continue
		}
		seen[diff] = struct{}{}
		result = append(result, diff)
	}
	sort.Slice(result, func(i, j int) bool {
		return difficultyOrder(result[i]) < difficultyOrder(result[j])
	})
	return result
}

func buildLookupMusic(musicInfo *masterdata.Music, builder *Builder, region renderregion.Value) *masterdata.Music {
	if musicInfo == nil {
		return nil
	}
	return &masterdata.Music{
		ID:                 musicInfo.ID,
		Seq:                musicInfo.Seq,
		ReleaseConditionID: musicInfo.ReleaseConditionID,
		Categories:         slices.Clone(musicInfo.Categories),
		Title:              builder.buildDisplayMusicTitle(musicInfo, region),
		Pronunciation:      musicInfo.Pronunciation,
		Lyricist:           musicInfo.Lyricist,
		Composer:           musicInfo.Composer,
		Arranger:           musicInfo.Arranger,
		DancerCount:        musicInfo.DancerCount,
		SelfDancerCount:    musicInfo.SelfDancerCount,
		AssetBundleName:    musicInfo.AssetBundleName,
		PublishedAt:        musicInfo.PublishedAt,
		DigitizedAt:        musicInfo.DigitizedAt,
		IsFullLength:       musicInfo.IsFullLength,
	}
}

func chartContainsBPM(parsed *parsedChartBPM, target float64) bool {
	if parsed == nil {
		return false
	}
	for _, event := range parsed.Events {
		if math.Abs(event.BPM-target) < 1e-9 {
			return true
		}
	}
	return false
}

func formatLookupBPMValue(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

type parsedChartBPM struct {
	MainBPM  float64
	Events   []BPMEvent
	BarCount int
	Duration float64
}

func parseChartBPM(ctx context.Context, path string) (*parsedChartBPM, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	score, barCount, err := readChartScore(ctx, path)
	if err != nil {
		return nil, err
	}
	finishParse := commandtrace.MeasureOperation(ctx, "music.chart_parse")
	defer finishParse()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bpmPalette := chartBPMPalette(score)
	rawEvents, err := chartRawBPMEvents(ctx, score, bpmPalette)
	if err != nil {
		return nil, err
	}
	if len(rawEvents) == 0 {
		return nil, fmt.Errorf("谱面中没有可用的 BPM 数据")
	}
	events := normalizeChartBPMEvents(rawEvents)
	totalDuration, mainBPM := applyChartBPMDurations(events, barCount)
	return &parsedChartBPM{MainBPM: mainBPM, Events: events, BarCount: barCount, Duration: totalDuration}, nil
}

func readChartScore(ctx context.Context, path string) (map[[2]string]string, int, error) {
	finishRead := commandtrace.MeasureOperation(ctx, "music.chart_read")
	defer finishRead()
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open chart file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	score := make(map[[2]string]string)
	barCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		match := susLinePattern.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if len(match) != 4 {
			continue
		}
		bar := strings.ToUpper(match[1])
		key := strings.ToUpper(match[2])
		value := strings.TrimSpace(match[3])
		score[[2]string{bar, key}] = value
		if barNumber, err := strconv.Atoi(bar); err == nil && barNumber+1 > barCount {
			barCount = barNumber + 1
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to read chart file: %w", err)
	}
	return score, barCount, nil
}

func chartBPMPalette(score map[[2]string]string) map[string]float64 {
	bpmPalette := make(map[string]float64)
	for token, value := range score {
		if token[0] != "BPM" {
			continue
		}
		bpmValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		bpmPalette[token[1]] = bpmValue
	}
	return bpmPalette
}

type rawChartBPMEvent struct {
	bar float64
	bpm float64
}

func chartRawBPMEvents(ctx context.Context, score map[[2]string]string, bpmPalette map[string]float64) ([]rawChartBPMEvent, error) {
	rawEvents := make([]rawChartBPMEvent, 0)
	for token, value := range score {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if token[1] != "08" {
			continue
		}
		rawEvents = append(rawEvents, chartRawBPMEventsForBar(token[0], value, bpmPalette)...)
	}
	return rawEvents, nil
}

func chartRawBPMEventsForBar(bar string, value string, bpmPalette map[string]float64) []rawChartBPMEvent {
	barNumber, err := strconv.Atoi(bar)
	length := len(value) / 2
	if err != nil || length == 0 {
		return nil
	}
	events := make([]rawChartBPMEvent, 0, length)
	for i := 0; i < length; i++ {
		bpmKey := strings.ToUpper(value[i*2 : (i+1)*2])
		bpmValue, ok := bpmPalette[bpmKey]
		if bpmKey == "00" || !ok {
			continue
		}
		events = append(events, rawChartBPMEvent{
			bar: float64(barNumber) + float64(i)/float64(length),
			bpm: bpmValue,
		})
	}
	return events
}

func normalizeChartBPMEvents(rawEvents []rawChartBPMEvent) []BPMEvent {
	sort.Slice(rawEvents, func(i, j int) bool {
		return rawEvents[i].bar < rawEvents[j].bar
	})

	events := make([]BPMEvent, 0, len(rawEvents))
	for _, item := range rawEvents {
		if len(events) > 0 && events[len(events)-1].BPM == item.bpm {
			continue
		}
		events = append(events, BPMEvent{
			Bar: item.bar,
			BPM: item.bpm,
		})
	}
	return events
}

func applyChartBPMDurations(events []BPMEvent, barCount int) (float64, float64) {
	durationByBPM := make(map[float64]float64)
	var totalDuration float64
	for i := range events {
		var nextBar float64
		if i+1 < len(events) {
			nextBar = events[i+1].Bar
		} else {
			nextBar = float64(barCount)
		}
		events[i].Duration = (nextBar - events[i].Bar) / events[i].BPM * 4 * 60
		totalDuration += events[i].Duration
		durationByBPM[events[i].BPM] += events[i].Duration
	}

	mainBPM := 0.0
	mainDuration := -1.0
	for bpm, duration := range durationByBPM {
		if duration > mainDuration {
			mainBPM = bpm
			mainDuration = duration
		}
	}

	return totalDuration, mainBPM
}

func buildBPMDifficultyCandidates(preferred string) []string {
	order := []string{"expert", "append", "master", "hard", "normal", "easy"}
	preferred = normalizeDifficulty(preferred)
	if preferred == "" {
		return order
	}
	result := []string{preferred}
	for _, item := range order {
		if item == preferred {
			continue
		}
		result = append(result, item)
	}
	return result
}

func difficultyOrder(difficulty string) int {
	switch normalizeDifficulty(difficulty) {
	case "easy":
		return 1
	case "normal":
		return 2
	case "hard":
		return 3
	case "expert":
		return 4
	case "master":
		return 5
	case "append":
		return 6
	default:
		return 99
	}
}
