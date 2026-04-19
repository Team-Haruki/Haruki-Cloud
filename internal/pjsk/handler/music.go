package handler

import (
	"fmt"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func (sekaiHandlers) MusicListHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music/list",
			Commands: []string{
				"/歌曲列表", "/歌曲一览", "/乐曲列表", "/乐曲一览", "/难度排行", "/定数表", "/歌曲定数", "/查乐曲", "/music-list", "/pjsk music list",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			if resultFilter, cleaned, ok := extractMusicListResultFilter(args); ok {
				args = cleaned
				params["result_filter"] = resultFilter
			}
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				args = cleaned
				params["difficulty"] = diff
			}
			if levelParams, cleaned, ok := extractMusicListLevelArgs(args); ok {
				args = cleaned
				for key, value := range levelParams {
					params[key] = value
				}
			}
			ctx.SetArgs(args)
			return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-list", params), nil
		},
	}, executeMusic)
}

func (sekaiHandlers) MusicRewardsHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music/rewards",
			Commands: []string{
				"/曲目奖励", "/歌曲奖励", "/music rewards", "/music-rewards", "/pjsk music rewards",
				"/打歌奖励", "/歌曲挖矿", "/打歌挖矿", "/pjsk 曲目奖励",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-rewards", params), nil
		},
	}, executeMusic)
}

func (sekaiHandlers) MusicProgressHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music/progress",
			Commands: []string{
				"/打歌进度", "/歌曲进度", "/打歌信息", "/pjsk进度", "/progress", "/music-progress", "/pjsk music progress", "/pjsk progress",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				ctx.SetArgs(cleaned)
				params["difficulty"] = diff
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-progress", params), nil
		},
	}, executeMusic)
}

func (sekaiHandlers) SongHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music",
			Commands: []string{
				"/pjsk song", "/pjsk music", "/song", "/music",
				"/查曲", "/查歌", "/歌曲", "/查歌曲",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			if query == "" {
				return nil, onebot11.NewReplayError("请输入要查询的歌曲名或ID")
			}
			if diff, cleaned := extractMusicDifficulty(query); diff != "" {
				ctx.SetArgs(cleaned)
				return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-detail", map[string]any{
					"difficulty": diff,
				}), nil
			}
			return makeCommandRequest(ctx, parser.ModuleMusic, "music-detail"), nil
		},
	}, executeMusic)
}

func (sekaiHandlers) NoteNumHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music/note-count",
			Commands: []string{
				"/pjsk note num", "/pjsk note count",
				"/物量", "/查物量",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			params := map[string]any{}
			if diff, cleaned := extractMusicDifficulty(args); diff != "" {
				args = cleaned
				params["difficulty"] = diff
			}
			args = strings.TrimSpace(args)
			noteCount, err := strconv.Atoi(args)
			if err != nil {
				return nil, onebot11.NewReplayError("请输入物量数值")
			}
			ctx.SetArgs(args)
			params["note_count"] = noteCount
			return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-note-count", params), nil
		},
	}, executeMusic)
}

func (sekaiHandlers) BPMHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music/bpm",
			Commands: []string{
				"/pjsk bpm", "/查bpm", "/查BPM",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			if query == "" {
				return nil, onebot11.NewReplayError("请输入要查询的 BPM 数值")
			}
			params := map[string]any{}
			if diff, cleaned := extractMusicDifficulty(query); diff != "" {
				query = cleaned
				params["difficulty"] = diff
			}
			query = strings.TrimSpace(query)
			bpmValue, err := strconv.ParseFloat(query, 64)
			if err != nil || bpmValue <= 0 {
				return nil, onebot11.NewReplayError("请输入正确的 BPM 数值")
			}
			ctx.SetArgs(query)
			params["bpm"] = bpmValue
			return makeCommandRequestWithParams(ctx, parser.ModuleMusic, "music-bpm", params), nil
		},
	}, executeMusic)
}

func (sekaiHandlers) MusicCoverHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "music/cover",
			Commands: []string{
				"/pjsk music cover",
				"/查曲绘", "/曲绘",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			query := strings.TrimSpace(ctx.GetArgs())
			if query == "" {
				return nil, fmt.Errorf("请输入要查询的歌曲名或ID")
			}
			return makeCommandRequest(ctx, parser.ModuleMusic, "music-cover"), nil
		},
	}, executeMusic)
}

func extractMusicListResultFilter(args string) (string, string, bool) {
	tokens := strings.Fields(strings.TrimSpace(args))
	for idx, token := range tokens {
		switch normalizeMusicListResultFilterToken(token) {
		case "not_ap":
			return "not_ap", joinMusicListTokensExcluding(tokens, idx), true
		case "not_fc":
			return "not_fc", joinMusicListTokensExcluding(tokens, idx), true
		case "not_clear":
			return "not_clear", joinMusicListTokensExcluding(tokens, idx), true
		}
	}
	return "", strings.TrimSpace(args), false
}

func normalizeMusicListResultFilterToken(token string) string {
	normalized := strings.ToLower(strings.TrimSpace(token))
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	switch normalized {
	case "未ap", "未allperfect", "未全perfect", "未全p", "notap":
		return "not_ap"
	case "未fc", "未fullcombo", "notfc":
		return "not_fc"
	case "未完成", "未clear", "未通关", "notclear":
		return "not_clear"
	default:
		return ""
	}
}

func extractMusicDifficulty(args string) (string, string) {
	return rendermusic.ExtractMusicDifficulty(args)
}

func extractMusicListLevelArgs(args string) (map[string]any, string, bool) {
	tokens := strings.Fields(strings.TrimSpace(args))
	for i := 0; i+2 < len(tokens); i++ {
		left, okLeft := parseMusicListExactLevelToken(tokens[i])
		right, okRight := parseMusicListExactLevelToken(tokens[i+2])
		if !okLeft || !okRight || !isMusicListRangeSeparatorToken(tokens[i+1]) {
			continue
		}
		values := []int{left, right}
		sort.Ints(values)
		return map[string]any{
			"level_min": values[0],
			"level_max": values[1],
		}, joinMusicListTokensExcluding(tokens, i, i+1, i+2), true
	}

	for i := 0; i+1 < len(tokens); i++ {
		left, okLeft := parseMusicListExactLevelToken(tokens[i])
		right, okRight := parseMusicListExactLevelToken(tokens[i+1])
		if !okLeft || !okRight {
			continue
		}
		values := []int{left, right}
		sort.Ints(values)
		return map[string]any{
			"level_min": values[0],
			"level_max": values[1],
		}, joinMusicListTokensExcluding(tokens, i, i+1), true
	}

	for i, token := range tokens {
		levelParams, ok := parseMusicListLevelToken(strings.TrimSpace(token))
		if !ok {
			continue
		}
		return levelParams, joinMusicListTokensExcluding(tokens, i), true
	}
	return nil, strings.TrimSpace(args), false
}

func parseMusicListLevelToken(token string) (map[string]any, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	if exact, ok := parseMusicListExactLevelToken(token); ok {
		return map[string]any{"level": exact}, true
	}

	if left, right, ok := parseMusicListRangeToken(token); ok {
		values := []int{left, right}
		sort.Ints(values)
		return map[string]any{
			"level_min": values[0],
			"level_max": values[1],
		}, true
	}

	switch {
	case strings.HasPrefix(token, "<="):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "<="))); err == nil && value > 0 {
			return map[string]any{"level_max": value}, true
		}
	case strings.HasPrefix(token, ">="):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, ">="))); err == nil && value > 0 {
			return map[string]any{"level_min": value}, true
		}
	case strings.HasPrefix(token, "<"):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "<"))); err == nil && value > 0 {
			return map[string]any{"level_max": value - 1}, value > 1
		}
	case strings.HasPrefix(token, ">"):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, ">"))); err == nil {
			return map[string]any{"level_min": value + 1}, value >= 0
		}
	case strings.HasPrefix(token, "="):
		if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "="))); err == nil && value > 0 {
			return map[string]any{"level": value}, true
		}
	}

	return nil, false
}

var musicListRangeSeparators = regexp.MustCompile(`^(\d+)\s*(?:-|~|～|,|，|\.\.|到|至)\s*(\d+)$`)

func parseMusicListRangeToken(token string) (int, int, bool) {
	normalized := strings.TrimSpace(token)
	for _, pair := range [][2]string{
		{"[", "]"},
		{"(", ")"},
		{"【", "】"},
		{"（", "）"},
	} {
		if strings.HasPrefix(normalized, pair[0]) && strings.HasSuffix(normalized, pair[1]) {
			normalized = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(normalized, pair[0]), pair[1]))
			break
		}
	}

	matches := musicListRangeSeparators.FindStringSubmatch(normalized)
	if len(matches) != 3 {
		return 0, 0, false
	}

	left, errLeft := strconv.Atoi(matches[1])
	right, errRight := strconv.Atoi(matches[2])
	if errLeft != nil || errRight != nil || left <= 0 || right <= 0 {
		return 0, 0, false
	}
	return left, right, true
}

func parseMusicListExactLevelToken(token string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func isMusicListRangeSeparatorToken(token string) bool {
	switch strings.TrimSpace(token) {
	case "-", "~", "～", ",", "，", "..", "到", "至":
		return true
	default:
		return false
	}
}

func joinMusicListTokensExcluding(tokens []string, skipIndexes ...int) string {
	if len(tokens) == 0 {
		return ""
	}
	skips := make(map[int]struct{}, len(skipIndexes))
	for _, idx := range skipIndexes {
		skips[idx] = struct{}{}
	}
	remaining := make([]string, 0, len(tokens))
	for idx, token := range tokens {
		if _, skip := skips[idx]; skip {
			continue
		}
		remaining = append(remaining, token)
	}
	return strings.TrimSpace(strings.Join(remaining, " "))
}

func executeMusic(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App == nil || rc.App.Music == nil {
		return nil, fmt.Errorf("music service unavailable: music controller is not configured")
	}
	musicCtrl := rc.App.Music.WithContext(rc.Ctx)
	if rc.App.Aliases != nil {
		musicCtrl.SetAliasResolver(rc.App.Aliases)
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "music-detail":
		q := rendermusic.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = musicCtrl.RenderMusicDetail(q)
		if err != nil {
			if ids := rendermusic.ExtractAmbiguousMusicIDs(err); len(ids) > 1 {
				items := make([]rendermusic.BriefListItemQuery, 0, len(ids))
				for _, musicID := range ids {
					items = append(items, rendermusic.BriefListItemQuery{MusicID: musicID})
				}
				return renderAmbiguousMusicDetailListMessages(rc, musicCtrl, q.Region, err, items)
			}
			return nil, err
		}
	case "music-list":
		_, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
		if suiteErr != nil {
			return nil, suiteErr
		}
		if suiteSnapshot != nil {
			musicCtrl = musicCtrl.WithSnapshot(suiteSnapshot)
		}
		q := rendermusic.ListQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if strings.TrimSpace(q.Keyword) == "" {
			q.Keyword = strings.TrimSpace(rc.Cmd.Query)
		}
		q.DetailedProfile = rc.GetDetailedProfile()
		if q.DetailedProfile == nil && suiteSnapshot != nil {
			q.DetailedProfile = suiteSnapshot.DetailedProfile(rc.Region)
		}
		data, err = musicCtrl.RenderMusicList(q)
	case "music-chart":
		q := rendermusic.ChartQuery{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if strings.TrimSpace(q.Style) == "" {
			q.Style = resolveRequesterHarukiUserChartStyle(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID)
		}
		data, err = musicCtrl.RenderMusicChart(q)
	case "music-progress":
		_, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
		if suiteErr != nil {
			return nil, suiteErr
		}
		q := rendermusic.ProgressQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		profile := rc.GetProfileCard()
		if profile == nil && suiteSnapshot != nil {
			profile = suiteSnapshot.ProfileCard(rc.Region)
		}
		if suiteSnapshot != nil {
			data, err = musicCtrl.RenderMusicProgressFromSnapshot(q, suiteSnapshot, profile)
		} else {
			data, err = musicCtrl.RenderMusicProgressFromSnapshot(q, nil, profile)
		}
	case "music-rewards":
		data, err = renderMusicRewards(rc)
	case "music-note-count":
		q := rendermusic.NoteCountQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		matches, resolveErr := musicCtrl.FindMusicChartsByNoteCount(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if len(matches) == 1 {
			data, err = renderSingleMusicLookupChart(
				musicCtrl,
				rc.Cmd.Region,
				matches[0].Music.ID,
				matches[0].Difficulty,
				resolveRequesterHarukiUserChartStyle(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID),
			)
			break
		}
		return renderNoteCountLookupListMessages(rc, musicCtrl, q, matches)
	case "music-cover":
		q := rendermusic.Query{Query: rc.Cmd.Query, Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := musicCtrl.ResolveMusicCover(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		image, imageErr := assetImageMessage(rc.Ctx, result.JacketPath, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		text := fmt.Sprintf("【%d】%s", result.Music.ID, result.Music.Title)
		return append(image, onebot11.Text(text)), nil
	case "music-bpm":
		q := rendermusic.BPMQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		if q.BPM <= 0 {
			if value, parseErr := strconv.ParseFloat(strings.TrimSpace(rc.Cmd.Query), 64); parseErr == nil {
				q.BPM = value
			}
		}
		matches, resolveErr := musicCtrl.FindMusicChartsByBPM(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if len(matches) == 1 {
			data, err = renderSingleMusicLookupChart(
				musicCtrl,
				rc.Cmd.Region,
				matches[0].Music.ID,
				matches[0].Difficulty,
				resolveRequesterHarukiUserChartStyle(rc.Ctx, rc.App, rc.Platform, rc.PlatformUserID),
			)
			break
		}
		return renderBPMLookupListMessages(rc, musicCtrl, q, matches)
	default:
		return nil, unsupportedModeError("music", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func renderSingleMusicLookupChart(musicCtrl *rendermusic.Controller, region string, musicID int, difficulty string, style string) ([]byte, error) {
	return musicCtrl.RenderMusicChart(rendermusic.ChartQuery{
		Query:      fmt.Sprintf("music%d", musicID),
		Region:     region,
		Difficulty: difficulty,
		Style:      style,
	})
}

func renderNoteCountLookupListMessages(rc *RequestContext, musicCtrl *rendermusic.Controller, query rendermusic.NoteCountQuery, matches []rendermusic.NoteCountMatch) (onebot11.Message, error) {
	items := make([]rendermusic.ListItemQuery, 0, len(matches))
	for _, item := range matches {
		if item.Music == nil {
			continue
		}
		items = append(items, rendermusic.ListItemQuery{
			MusicID:    item.Music.ID,
			Difficulty: item.Difficulty,
			Level:      item.PlayLevel,
		})
	}
	return renderMusicLookupListMessages(rc, musicCtrl, rc.Cmd.Region, "物量", strconv.Itoa(query.NoteCount), query.Difficulty, query.Difficulty, items)
}

func renderBPMLookupListMessages(rc *RequestContext, musicCtrl *rendermusic.Controller, query rendermusic.BPMQuery, matches []rendermusic.BPMMatch) (onebot11.Message, error) {
	uniqueMatches := dedupeBPMMatchesByMusic(matches)
	items := make([]rendermusic.BriefListItemQuery, 0, len(uniqueMatches))
	for _, item := range uniqueMatches {
		if item.Music == nil {
			continue
		}
		items = append(items, rendermusic.BriefListItemQuery{
			MusicID: item.Music.ID,
		})
	}
	return renderMusicBriefLookupListMessages(rc, musicCtrl, rc.Cmd.Region, "BPM", formatMusicBPM(query.BPM), items)
}

func renderMusicLookupListMessages(rc *RequestContext, musicCtrl *rendermusic.Controller, region string, prefix string, value string, requestDifficulty string, titleDifficulty string, items []rendermusic.ListItemQuery) (onebot11.Message, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}
	data, err := musicCtrl.RenderMusicList(rendermusic.ListQuery{
		Items:       items,
		Difficulty:  requestDifficulty,
		Region:      region,
		Title:       stringPtr(buildMusicLookupListTitle(prefix, value, titleDifficulty)),
		TitleShadow: true,
	})
	if err != nil {
		return nil, err
	}
	image, err := rc.ImageMessage(data)
	if err != nil {
		return nil, err
	}
	return image, nil
}

func renderMusicBriefLookupListMessages(rc *RequestContext, musicCtrl *rendermusic.Controller, region string, prefix string, value string, items []rendermusic.BriefListItemQuery) (onebot11.Message, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}
	data, err := musicCtrl.RenderMusicBriefList(rendermusic.BriefListQuery{
		Items:       items,
		Region:      region,
		Title:       stringPtr(buildMusicLookupListTitle(prefix, value, "")),
		TitleShadow: true,
	})
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func renderAmbiguousMusicDetailListMessages(rc *RequestContext, musicCtrl *rendermusic.Controller, region string, sourceErr error, items []rendermusic.BriefListItemQuery) (onebot11.Message, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no music matched the current filters")
	}
	title := buildAmbiguousMusicDetailListTitle(sourceErr)
	data, err := musicCtrl.RenderMusicBriefList(rendermusic.BriefListQuery{
		Items:       items,
		Region:      region,
		Title:       stringPtr(title),
		TitleShadow: true,
	})
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}

func buildAmbiguousMusicDetailListTitle(sourceErr error) string {
	const fallbackTitle = "匹配到多个歌曲，请使用查歌 <id> 查询："
	if sourceErr == nil {
		return fallbackTitle
	}
	line := strings.TrimSpace(strings.Split(sourceErr.Error(), "\n")[0])
	line = strings.TrimPrefix(line, "failed to search music: ")
	line = strings.ReplaceAll(line, "music<id>", "查歌 <id>")
	line = strings.ReplaceAll(line, "请改用", "请使用")
	line = strings.ReplaceAll(line, "请使用 查歌", "请使用查歌")
	line = strings.TrimSpace(line)
	if line == "" {
		return fallbackTitle
	}
	if strings.Contains(line, "匹配到多个歌曲") {
		return fallbackTitle
	}
	return line
}

func dedupeBPMMatchesByMusic(matches []rendermusic.BPMMatch) []rendermusic.BPMMatch {
	if len(matches) == 0 {
		return nil
	}
	result := make([]rendermusic.BPMMatch, 0, len(matches))
	seen := make(map[int]struct{}, len(matches))
	for _, match := range matches {
		if match.Music == nil {
			continue
		}
		if _, ok := seen[match.Music.ID]; ok {
			continue
		}
		seen[match.Music.ID] = struct{}{}
		result = append(result, match)
	}
	return result
}

func buildMusicLookupListTitle(prefix string, value string, difficulty string) string {
	title := fmt.Sprintf("%s %s 匹配结果", strings.TrimSpace(prefix), strings.TrimSpace(value))
	if diffLabel := formatMusicDifficultyLabel(difficulty); diffLabel != "" {
		title = fmt.Sprintf("%s %s %s 匹配结果", strings.TrimSpace(prefix), strings.TrimSpace(value), diffLabel)
	}
	return strings.TrimSpace(title)
}

func formatMusicDifficultyLabel(difficulty string) string {
	switch strings.ToLower(strings.TrimSpace(difficulty)) {
	case "easy":
		return "EASY"
	case "normal":
		return "NORMAL"
	case "hard":
		return "HARD"
	case "expert":
		return "EXPERT"
	case "master":
		return "MASTER"
	case "append":
		return "APPEND"
	default:
		return ""
	}
}

func formatMusicBPM(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func renderMusicRewards(rc *RequestContext) ([]byte, error) {
	musicCtrl := rc.App.Music.WithContext(rc.Ctx)
	if rc.App.Aliases != nil {
		musicCtrl.SetAliasResolver(rc.App.Aliases)
	}
	_, snapshot, err := rc.requireVisibleSuiteSnapshot()
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, onebot11.NewReplayError(ErrMsgSuiteDataNotFound)
	}

	detailQuery := rendermusic.RewardsDetailQuery{Region: rc.Cmd.Region}
	mergeParams(rc.Cmd.Params, &detailQuery)
	detailQuery.Profile = rc.GetProfileCard()
	if _, buildErr := musicCtrl.BuildMusicRewardsDetailRequestFromSnapshot(detailQuery, snapshot); buildErr != nil {
		return nil, onebot11.NewReplayError(ErrMsgSuiteDataNotFound)
	}
	return musicCtrl.RenderMusicRewardsDetailFromSnapshot(detailQuery, snapshot)
}
