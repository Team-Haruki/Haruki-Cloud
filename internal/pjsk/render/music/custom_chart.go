package music

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	TagIDs              []int
}

type customChartStats struct {
	NoteCount int
	BPM       string
}

type customChartNote struct {
	ID                   int
	Ticks                int
	LaneStart            int
	LaneEnd              int
	Category             int
	NoteBaseType         int
	PreviousConnectionID int
	NextConnectionID     int
	Direction            int
	Critical             bool
	IsSkip               bool
	hasTicks             bool
	hasPrevious          bool
	hasNext              bool
}

type customChartConvertedNote struct {
	ID       int
	ParentID int
	Type     customChartNoteType
	Tick     int
	Lane     int
	Width    int
	Critical bool
	Friction bool
	Flick    customChartFlickType
}

type customChartHoldStep struct {
	ID   int
	Type customChartHoldStepType
}

type customChartHold struct {
	Start     customChartHoldStep
	Steps     []customChartHoldStep
	End       int
	StartType customChartHoldNoteType
	EndType   customChartHoldNoteType
}

func (h customChartHold) isGuide() bool {
	return h.StartType == customChartHoldNoteGuide || h.EndType == customChartHoldNoteGuide
}

type customChartScore struct {
	notes     map[int]customChartConvertedNote
	holdNotes map[int]customChartHold
	nextID    int
}

func newCustomChartScore() customChartScore {
	return customChartScore{
		notes:     make(map[int]customChartConvertedNote),
		holdNotes: make(map[int]customChartHold),
		nextID:    1,
	}
}

func (s *customChartScore) allocateID() int {
	id := s.nextID
	s.nextID++
	return id
}

type customChartNoteType int

const (
	customChartNoteTap customChartNoteType = iota
	customChartNoteHold
	customChartNoteHoldMid
	customChartNoteHoldEnd
)

type customChartFlickType int

const (
	customChartFlickNone customChartFlickType = iota
	customChartFlickDefault
	customChartFlickLeft
	customChartFlickRight
)

type customChartHoldStepType int

const (
	customChartHoldStepNormal customChartHoldStepType = iota
	customChartHoldStepHidden
	customChartHoldStepSkip
)

type customChartHoldNoteType int

const (
	customChartHoldNoteNormal customChartHoldNoteType = iota
	customChartHoldNoteHidden
	customChartHoldNoteGuide
)

type customChartSlideKind int

const (
	customChartSlideStart customChartSlideKind = iota
	customChartSlideEnd
	customChartSlideRelay
	customChartSlideInvisible
)

type customMusicScoreTagResolver interface {
	GetCustomMusicScoreTagNames(tagIDs []int) []string
}

const (
	// Long-note combo ticks advance every 240 custom-score ticks.
	customChartComboTickInterval = 240
	customChartMaxLane           = 11
	customChartMaxDecodedBytes   = 8 << 20
	customChartMaxEncodedBytes   = 12 << 20
)

func IsCustomChartIDQuery(query string) bool {
	_, ok := customChartIDFromQuery(query)
	return ok
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
		Tags:                resolveCustomChartTags(source, entry.TagIDs),
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
		TagIDs:              append([]int(nil), value.CustomMusicScoreTags...),
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
		NoteList []customChartNote `json:"NoteList"`
	}
	if err := stdjson.Unmarshal(chartRaw, &chart); err != nil {
		return customChartStats{}
	}
	return customChartStats{
		NoteCount: calculateCustomChartComboCount(chart.NoteList),
		BPM:       formatCustomChartBPMs(chart.MusicScoreEventDataList),
	}
}

func (n *customChartNote) UnmarshalJSON(data []byte) error {
	var raw map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &raw); err != nil {
		return err
	}

	n.ID, _ = customChartRawInt(raw, "id", 0)
	if ticks, ok := customChartRawInt(raw, "ticks", 0); ok {
		n.Ticks = ticks
		n.hasTicks = true
	}
	n.LaneStart, _ = customChartRawInt(raw, "laneStart", 0)
	n.LaneEnd, _ = customChartRawInt(raw, "laneEnd", 0)
	n.Category, _ = customChartRawInt(raw, "category", 0)
	n.NoteBaseType, _ = customChartRawInt(raw, "noteBaseType", 0)
	if previous, ok := customChartRawInt(raw, "previousConnectionId", -1); ok {
		n.PreviousConnectionID = previous
		n.hasPrevious = true
	} else {
		n.PreviousConnectionID = -1
	}
	if next, ok := customChartRawInt(raw, "nextConnectionId", -1); ok {
		n.NextConnectionID = next
		n.hasNext = true
	} else {
		n.NextConnectionID = -1
	}
	n.Direction, _ = customChartRawInt(raw, "direction", 0)
	n.Critical, _ = customChartRawBool(raw, "type", false)
	n.IsSkip, _ = customChartRawBool(raw, "isSkip", false)
	return nil
}

func (n customChartNote) validForCombo() bool {
	return n.ID != 0 && n.hasTicks && n.hasPrevious && n.hasNext
}

func calculateCustomChartComboCount(notes []customChartNote) int {
	if len(notes) == 0 {
		return 0
	}

	sorted := make([]customChartNote, 0, len(notes))
	for _, note := range notes {
		if note.validForCombo() {
			sorted = append(sorted, note)
		}
	}
	if len(sorted) == 0 {
		return 0
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Ticks != sorted[j].Ticks {
			return sorted[i].Ticks < sorted[j].Ticks
		}
		if sorted[i].LaneStart != sorted[j].LaneStart {
			return sorted[i].LaneStart < sorted[j].LaneStart
		}
		return sorted[i].ID < sorted[j].ID
	})

	byID := make(map[int]*customChartNote, len(sorted))
	for i := range sorted {
		byID[sorted[i].ID] = &sorted[i]
	}

	score := newCustomChartScore()
	connectedIDs := make(map[int]struct{}, len(sorted))
	chains := buildCustomChartChains(sorted, byID, connectedIDs)
	for _, chain := range chains {
		addCustomChartChain(&score, chain)
	}
	for i := range sorted {
		note := &sorted[i]
		if _, ok := connectedIDs[note.ID]; ok {
			continue
		}
		addCustomChartTap(&score, *note, false)
	}

	return calculateCustomChartScoreComboCount(score)
}

func buildCustomChartChains(notes []customChartNote, byID map[int]*customChartNote, connectedIDs map[int]struct{}) [][]*customChartNote {
	chains := make([][]*customChartNote, 0)
	for i := range notes {
		note := &notes[i]
		if _, ok := connectedIDs[note.ID]; ok {
			continue
		}
		if note.NextConnectionID == -1 && note.PreviousConnectionID == -1 {
			continue
		}
		if note.PreviousConnectionID != -1 {
			continue
		}
		chain := collectCustomChartChain(note, byID, connectedIDs)
		if len(chain) > 0 {
			chains = append(chains, chain)
		}
	}
	return appendUnconnectedCustomChartChains(chains, notes, connectedIDs)
}

func collectCustomChartChain(first *customChartNote, byID map[int]*customChartNote, connectedIDs map[int]struct{}) []*customChartNote {
	chain := make([]*customChartNote, 0)
	for current := first; current != nil; current = byID[current.NextConnectionID] {
		if _, ok := connectedIDs[current.ID]; ok {
			break
		}
		chain = append(chain, current)
		connectedIDs[current.ID] = struct{}{}
		if current.NextConnectionID == -1 {
			break
		}
	}
	return chain
}

func appendUnconnectedCustomChartChains(chains [][]*customChartNote, notes []customChartNote, connectedIDs map[int]struct{}) [][]*customChartNote {
	for i := range notes {
		note := &notes[i]
		if _, ok := connectedIDs[note.ID]; ok {
			continue
		}
		if note.NextConnectionID != -1 || note.PreviousConnectionID != -1 {
			chains = append(chains, []*customChartNote{note})
			connectedIDs[note.ID] = struct{}{}
		}
	}
	return chains
}

func addCustomChartChain(score *customChartScore, rawChain []*customChartNote) {
	chain := removeAdjacentCustomChartVisibleRelayDuplicates(rawChain)
	if len(chain) < 2 || chain[0] == nil || chain[len(chain)-1] == nil {
		return
	}

	decoration := customChartChainHasDecoration(chain)
	startID := score.allocateID()
	hold := customChartHold{}
	for index, raw := range chain {
		if raw == nil {
			continue
		}
		isFirst := index == 0
		isLast := index+1 == len(chain)
		kind := customChartSlideKindFor(*raw, isLast)

		noteID := startID
		if !isFirst {
			noteID = score.allocateID()
		}
		note := customChartConvertedNote{
			ID:       noteID,
			ParentID: startID,
			Tick:     raw.Ticks,
			Lane:     customChartLane(*raw),
			Width:    customChartWidth(*raw),
			Critical: raw.Critical,
			Friction: customChartIsTraceNote(*raw),
			Flick:    customChartFlickTypeFor(*raw),
		}
		if isFirst {
			note.ParentID = -1
			note.Type = customChartNoteHold
			hold.Start = customChartHoldStep{ID: note.ID, Type: customChartHoldStepNormal}
			hold.StartType = customChartEndpointTypeFor(*raw, decoration)
		} else if isLast {
			note.Type = customChartNoteHoldEnd
			hold.End = note.ID
			hold.EndType = customChartEndpointTypeFor(*raw, decoration)
		} else {
			note.Type = customChartNoteHoldMid
			hold.Steps = append(hold.Steps, customChartHoldStep{
				ID:   note.ID,
				Type: customChartStepTypeFor(*raw, kind),
			})
		}
		score.notes[note.ID] = note
	}

	if hold.Start.ID == 0 || hold.End == 0 {
		return
	}
	sort.SliceStable(hold.Steps, func(i, j int) bool {
		left := score.notes[hold.Steps[i].ID]
		right := score.notes[hold.Steps[j].ID]
		if left.Tick != right.Tick {
			return left.Tick < right.Tick
		}
		return left.Lane < right.Lane
	})
	score.holdNotes[startID] = hold
}

func addCustomChartTap(score *customChartScore, raw customChartNote, forceCritical bool) {
	if customChartIsCancelNote(raw) {
		return
	}
	id := score.allocateID()
	score.notes[id] = customChartConvertedNote{
		ID:       id,
		ParentID: -1,
		Type:     customChartNoteTap,
		Tick:     raw.Ticks,
		Lane:     customChartLane(raw),
		Width:    customChartWidth(raw),
		Critical: forceCritical || raw.Critical,
		Friction: customChartIsTraceNote(raw),
		Flick:    customChartFlickTypeFor(raw),
	}
}

func calculateCustomChartScoreComboCount(score customChartScore) int {
	holdStepTypesByID := customChartHoldStepTypes(score)
	seen := make(map[string]struct{}, len(score.notes)*2)
	total := countCustomChartScoreNotes(score, holdStepTypesByID, seen)
	return total + countCustomChartHoldTicks(score, seen)
}

func customChartHoldStepTypes(score customChartScore) map[int]customChartHoldStepType {
	typesByID := make(map[int]customChartHoldStepType, len(score.notes))
	for _, hold := range score.holdNotes {
		for _, step := range hold.Steps {
			typesByID[step.ID] = step.Type
		}
	}
	return typesByID
}

func countCustomChartScoreNotes(score customChartScore, holdStepTypesByID map[int]customChartHoldStepType, seen map[string]struct{}) int {
	total := 0
	for _, note := range score.notes {
		hold, hasHold := customChartHoldForNote(score, note)
		if !customChartNoteCounts(note, hold, hasHold, holdStepTypesByID) {
			continue
		}
		key := customChartComboDedupKey(note)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		total++
	}
	return total
}

func customChartNoteCounts(note customChartConvertedNote, hold customChartHold, hasHold bool, holdStepTypesByID map[int]customChartHoldStepType) bool {
	if customChartNoteRequiresHold(note.Type) && !hasHold {
		return false
	}
	if hasHold && hold.isGuide() {
		return false
	}
	if note.Type == customChartNoteHold && hasHold && hold.StartType != customChartHoldNoteNormal {
		return false
	}
	if note.Type == customChartNoteHoldEnd && hasHold && hold.EndType != customChartHoldNoteNormal {
		return false
	}
	stepType, hasStepType := holdStepTypesByID[note.ID]
	return note.Type != customChartNoteHoldMid || !hasStepType || stepType != customChartHoldStepHidden
}

func countCustomChartHoldTicks(score customChartScore, seen map[string]struct{}) int {
	total := 0
	for holdID, hold := range score.holdNotes {
		halfBeatTick, endTick, ok := customChartHoldTickBounds(score, holdID, hold)
		if !ok {
			continue
		}
		for tick := halfBeatTick; tick < endTick; tick += customChartComboTickInterval {
			key := customChartHoldHalfBeatDedupKey(holdID, hold, score, tick)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			total++
		}
	}
	return total
}

func customChartHoldTickBounds(score customChartScore, holdID int, hold customChartHold) (int, int, bool) {
	if hold.isGuide() {
		return 0, 0, false
	}
	start, startOK := score.notes[holdID]
	end, endOK := score.notes[hold.End]
	if !startOK || !endOK {
		return 0, 0, false
	}
	halfBeatTick := start.Tick + customChartComboTickInterval
	if remainder := halfBeatTick % customChartComboTickInterval; remainder != 0 {
		halfBeatTick -= remainder
	}
	if halfBeatTick == start.Tick || halfBeatTick == end.Tick {
		return 0, 0, false
	}
	endTick := end.Tick
	if remainder := endTick % customChartComboTickInterval; remainder != 0 {
		endTick += customChartComboTickInterval - remainder
	}
	return halfBeatTick, endTick, true
}

func customChartRawInt(raw map[string]stdjson.RawMessage, key string, fallback int) (int, bool) {
	value, ok := raw[key]
	if !ok || len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return fallback, false
	}
	var decoded any
	if err := stdjson.Unmarshal(value, &decoded); err != nil {
		return fallback, false
	}
	if parsed, ok := customChartIntValue(decoded); ok {
		return parsed, true
	}
	return fallback, false
}

func customChartRawBool(raw map[string]stdjson.RawMessage, key string, fallback bool) (bool, bool) {
	value, ok := raw[key]
	if !ok || len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return fallback, false
	}
	var decoded any
	if err := stdjson.Unmarshal(value, &decoded); err != nil {
		return fallback, false
	}
	switch v := decoded.(type) {
	case bool:
		return v, true
	case float64:
		return v != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return fallback, false
}

func customChartLane(note customChartNote) int {
	return clampCustomChartInt(note.LaneStart, 0, customChartMaxLane)
}

func customChartWidth(note customChartNote) int {
	return clampCustomChartInt(note.LaneEnd-note.LaneStart+1, 1, customChartMaxLane+1)
}

func clampCustomChartInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func removeAdjacentCustomChartVisibleRelayDuplicates(chain []*customChartNote) []*customChartNote {
	filtered := make([]*customChartNote, 0, len(chain))
	for index, note := range chain {
		var next *customChartNote
		if index+1 < len(chain) {
			next = chain[index+1]
		}
		if note != nil && next != nil &&
			customChartIsVisibleRelaySlideNote(*note) &&
			customChartIsVisibleRelaySlideNote(*next) &&
			absCustomChartInt(next.Ticks-note.Ticks) <= 1 {
			continue
		}
		filtered = append(filtered, note)
	}
	return filtered
}

func absCustomChartInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func customChartChainHasDecoration(chain []*customChartNote) bool {
	for _, note := range chain {
		if note != nil && customChartIsDecorationSlideNote(*note) {
			return true
		}
	}
	return false
}

func customChartIsVisibleRelaySlideNote(note customChartNote) bool {
	return note.NoteBaseType == 5 || note.Category == 2
}

func customChartIsVisibleRelayAttachment(note customChartNote) bool {
	return customChartIsVisibleRelaySlideNote(note) && note.IsSkip
}

func customChartIsDecorationSlideNote(note customChartNote) bool {
	return note.Category == 9 || note.NoteBaseType == 10 || note.NoteBaseType == 13
}

func customChartSlideKindFor(note customChartNote, isLast bool) customChartSlideKind {
	if isLast {
		return customChartSlideEnd
	}
	base := note.NoteBaseType
	switch {
	case base == 2 || base == 8 || base == 9 || base == 10 || note.Category == 6:
		return customChartSlideStart
	case base == 1 || base == 3 || base == 11 || base == 12 || base == 13:
		return customChartSlideEnd
	case base == 6 || base == 14 || note.Category == 11:
		return customChartSlideInvisible
	default:
		return customChartSlideRelay
	}
}

func customChartIsCancelNote(note customChartNote) bool {
	base := note.NoteBaseType
	return base == 9 || base == 12 || base == 10 || base == 13
}

func customChartIsTraceNote(note customChartNote) bool {
	base := note.NoteBaseType
	return base == 4 || base == 8 || base == 11 || note.Category == 4 || note.Category == 6 || note.Category == 8
}

func customChartIsTraceFlickNote(note customChartNote) bool {
	return note.NoteBaseType == 4 || note.Category == 8
}

func customChartIsFlickNote(note customChartNote) bool {
	return note.NoteBaseType == 3 || note.Category == 3
}

func customChartFlickTypeFor(note customChartNote) customChartFlickType {
	if !customChartIsFlickNote(note) && !customChartIsTraceFlickNote(note) && note.Direction != 1 && note.Direction != 2 {
		return customChartFlickNone
	}
	switch note.Direction {
	case 1:
		return customChartFlickLeft
	case 2:
		return customChartFlickRight
	default:
		return customChartFlickDefault
	}
}

func customChartStepTypeFor(note customChartNote, kind customChartSlideKind) customChartHoldStepType {
	if kind == customChartSlideInvisible {
		return customChartHoldStepHidden
	}
	if customChartIsVisibleRelayAttachment(note) {
		return customChartHoldStepSkip
	}
	return customChartHoldStepNormal
}

func customChartEndpointTypeFor(note customChartNote, decoration bool) customChartHoldNoteType {
	if decoration {
		return customChartHoldNoteGuide
	}
	if customChartIsCancelNote(note) {
		return customChartHoldNoteHidden
	}
	return customChartHoldNoteNormal
}

func customChartNoteRequiresHold(noteType customChartNoteType) bool {
	return noteType == customChartNoteHold || noteType == customChartNoteHoldMid || noteType == customChartNoteHoldEnd
}

func customChartHoldForNote(score customChartScore, note customChartConvertedNote) (customChartHold, bool) {
	switch note.Type {
	case customChartNoteHold:
		hold, ok := score.holdNotes[note.ID]
		return hold, ok
	case customChartNoteHoldMid, customChartNoteHoldEnd:
		hold, ok := score.holdNotes[note.ParentID]
		return hold, ok
	default:
		return customChartHold{}, false
	}
}

func customChartComboDedupKey(note customChartConvertedNote) string {
	return fmt.Sprintf("%d|%d|%d|%d|%d|%d|%d",
		note.Type,
		note.Tick,
		note.Lane,
		note.Width,
		customChartBoolInt(note.Critical),
		customChartBoolInt(note.Friction),
		note.Flick,
	)
}

func customChartHoldHalfBeatDedupKey(holdID int, hold customChartHold, score customChartScore, tick int) string {
	holdStart := score.notes[holdID]
	holdEnd := score.notes[hold.End]
	var builder strings.Builder
	builder.WriteString(strconv.Itoa(tick))
	for _, value := range []int{
		holdStart.Tick,
		holdStart.Lane,
		holdStart.Width,
		holdEnd.Tick,
		holdEnd.Lane,
		holdEnd.Width,
		customChartBoolInt(holdStart.Critical),
		customChartBoolInt(holdStart.Friction),
	} {
		builder.WriteByte('|')
		builder.WriteString(strconv.Itoa(value))
	}
	for _, step := range hold.Steps {
		stepNote := score.notes[step.ID]
		builder.WriteByte('|')
		builder.WriteString(strconv.Itoa(stepNote.Tick))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(stepNote.Lane))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(stepNote.Width))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(int(step.Type)))
	}
	return builder.String()
}

func customChartBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

func customChartIntValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(math.Round(v)), true
	case stdjson.Number:
		if parsed, err := v.Int64(); err == nil {
			return int(parsed), true
		}
		parsed, err := v.Float64()
		return int(math.Round(parsed)), err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func resolveCustomChartTags(source DataSource, tagIDs []int) []string {
	if len(tagIDs) == 0 {
		return nil
	}
	if source != nil {
		if resolver, ok := any(source).(customMusicScoreTagResolver); ok {
			return compactCustomChartStrings(resolver.GetCustomMusicScoreTagNames(tagIDs))
		}
	}
	return nil
}

func compactCustomChartStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
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
	if len(raw) > customChartMaxEncodedBytes {
		return nil, fmt.Errorf("自制谱面数据超过 %d 字节限制", customChartMaxEncodedBytes)
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
	decoded, err := io.ReadAll(io.LimitReader(reader, customChartMaxDecodedBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("读取自制谱面 JSON 失败: %w", err)
	}
	if len(decoded) > customChartMaxDecodedBytes {
		return nil, true, fmt.Errorf("自制谱面解压后超过 %d 字节限制", customChartMaxDecodedBytes)
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
	if len(raw) > customChartMaxDecodedBytes {
		return nil, fmt.Errorf("自制谱面 JSON 超过 %d 字节限制", customChartMaxDecodedBytes)
	}
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
