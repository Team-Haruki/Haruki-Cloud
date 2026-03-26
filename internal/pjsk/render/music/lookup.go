package music

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type NoteCountMatch struct {
	Music          *masterdata.Music
	Difficulty     string
	PlayLevel      int
	TotalNoteCount int
}

type CoverResult struct {
	Music      *masterdata.Music
	JacketPath string
}

type BPMEvent struct {
	Bar      float64
	BPM      float64
	Duration float64
}

type BPMResult struct {
	Music      *masterdata.Music
	JacketPath string
	Difficulty string
	MainBPM    float64
	Events     []BPMEvent
	BarCount   int
	Duration   float64
}

type noteCountFinder interface {
	FindMusicDifficultiesByNoteCount(noteCount int) ([]*masterdata.MusicDifficulty, error)
}

var susLinePattern = regexp.MustCompile(`^#([A-Za-z0-9]{3})([A-Za-z0-9]{2})\s*:\s*(\S+)`)

func (c *Controller) FindMusicChartsByNoteCount(query NoteCountQuery) ([]NoteCountMatch, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	if query.NoteCount <= 0 {
		return nil, fmt.Errorf("物量必须大于 0")
	}

	_, source, _, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}

	matches := make([]NoteCountMatch, 0)
	if finder, ok := source.(noteCountFinder); ok {
		items, err := finder.FindMusicDifficultiesByNoteCount(query.NoteCount)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if _, hidden := hiddenMusicIDs[item.MusicID]; hidden {
				continue
			}
			musicInfo, err := source.GetMusicByID(item.MusicID)
			if err != nil || musicInfo == nil {
				continue
			}
			matches = append(matches, NoteCountMatch{
				Music:          musicInfo,
				Difficulty:     normalizeDifficulty(item.MusicDifficulty),
				PlayLevel:      item.PlayLevel,
				TotalNoteCount: item.TotalNoteCount,
			})
		}
	} else {
		for _, musicInfo := range source.GetMusics() {
			if musicInfo == nil {
				continue
			}
			if _, hidden := hiddenMusicIDs[musicInfo.ID]; hidden {
				continue
			}
			difficulties, err := source.GetMusicDifficulties(musicInfo.ID)
			if err != nil {
				continue
			}
			for _, item := range difficulties {
				if item == nil || item.TotalNoteCount != query.NoteCount {
					continue
				}
				matches = append(matches, NoteCountMatch{
					Music:          musicInfo,
					Difficulty:     normalizeDifficulty(item.MusicDifficulty),
					PlayLevel:      item.PlayLevel,
					TotalNoteCount: item.TotalNoteCount,
				})
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("没有找到物量为 %d 的谱面", query.NoteCount)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Music.ID == matches[j].Music.ID {
			return difficultyOrder(matches[i].Difficulty) < difficultyOrder(matches[j].Difficulty)
		}
		return matches[i].Music.ID < matches[j].Music.ID
	})
	return matches, nil
}

func (c *Controller) ResolveMusicCover(query Query) (*CoverResult, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	musicInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}

	jacketPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName)
	if localPath := c.resolveLocalMusicJacket(musicInfo.AssetBundleName); localPath != "" {
		jacketPath = localPath
	}
	if strings.TrimSpace(jacketPath) == "" {
		return nil, fmt.Errorf("music %d does not have jacket asset", musicInfo.ID)
	}

	return &CoverResult{
		Music: &masterdata.Music{
			ID:                 musicInfo.ID,
			Seq:                musicInfo.Seq,
			ReleaseConditionID: musicInfo.ReleaseConditionID,
			Categories:         append([]string(nil), musicInfo.Categories...),
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
		},
		JacketPath: jacketPath,
	}, nil
}

func (c *Controller) ResolveMusicBPM(query Query) (*BPMResult, error) {
	if c == nil {
		return nil, fmt.Errorf("music controller is not configured")
	}
	parser := NewParser(c.nicknames)
	if preferred, cleaned := parser.extractDiff(query.Query); preferred != "" && strings.TrimSpace(query.Difficulty) == "" {
		query.Difficulty = preferred
		query.Query = cleaned
	}

	region, source, builder, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, parser)
	musicInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search music: %w", err)
	}

	difficulties := buildBPMDifficultyCandidates(query.Difficulty)
	var (
		chartPath string
		diffUsed  string
	)
	for _, difficulty := range difficulties {
		chartPath = c.resolveLocalChartPath(musicInfo.ID, difficulty)
		if chartPath == "" {
			continue
		}
		diffUsed = difficulty
		break
	}
	if chartPath == "" {
		return nil, fmt.Errorf("当前环境没有可读取的本地谱面文件，无法查询 BPM")
	}

	parsed, err := parseChartBPM(chartPath)
	if err != nil {
		return nil, err
	}

	jacketPath := builder.BuildMusicJacketPath(musicInfo.AssetBundleName)
	if localPath := c.resolveLocalMusicJacket(musicInfo.AssetBundleName); localPath != "" {
		jacketPath = localPath
	}

	return &BPMResult{
		Music: &masterdata.Music{
			ID:                 musicInfo.ID,
			Seq:                musicInfo.Seq,
			ReleaseConditionID: musicInfo.ReleaseConditionID,
			Categories:         append([]string(nil), musicInfo.Categories...),
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
		},
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
		filepath.Join("music", "jacket", assetName+"_rip", assetName+".png"),
	)
}

func (c *Controller) resolveLocalChartPath(musicID int, difficulty string) string {
	if c == nil || c.assets == nil || musicID <= 0 || strings.TrimSpace(difficulty) == "" {
		return ""
	}
	diff := normalizeDifficulty(difficulty)
	return c.assets.FirstExisting(
		filepath.Join("music", "music_score", fmt.Sprintf("%04d_01", musicID), diff+".txt"),
		filepath.Join("music", "music_score", fmt.Sprintf("%04d_01_rip", musicID), diff),
		filepath.Join("music", "music_score", fmt.Sprintf("%04d_01_rip", musicID), diff+".txt"),
	)
}

type parsedChartBPM struct {
	MainBPM  float64
	Events   []BPMEvent
	BarCount int
	Duration float64
}

func parseChartBPM(path string) (*parsedChartBPM, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open chart file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	score := make(map[[2]string]string)
	barCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
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
		return nil, fmt.Errorf("failed to read chart file: %w", err)
	}

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

	type rawEvent struct {
		bar float64
		bpm float64
	}
	rawEvents := make([]rawEvent, 0)
	for token, value := range score {
		if token[1] != "08" {
			continue
		}
		barNumber, err := strconv.Atoi(token[0])
		if err != nil {
			continue
		}
		length := len(value) / 2
		if length == 0 {
			continue
		}
		for i := 0; i < length; i++ {
			bpmKey := strings.ToUpper(value[i*2 : (i+1)*2])
			if bpmKey == "00" {
				continue
			}
			bpmValue, ok := bpmPalette[bpmKey]
			if !ok {
				continue
			}
			rawEvents = append(rawEvents, rawEvent{
				bar: float64(barNumber) + float64(i)/float64(length),
				bpm: bpmValue,
			})
		}
	}
	if len(rawEvents) == 0 {
		return nil, fmt.Errorf("谱面中没有可用的 BPM 数据")
	}

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

	return &parsedChartBPM{
		MainBPM:  mainBPM,
		Events:   events,
		BarCount: barCount,
		Duration: totalDuration,
	}, nil
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
