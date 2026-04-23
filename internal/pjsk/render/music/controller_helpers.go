package music

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

func (c *Controller) currentSnapshot() snapshot.Snapshot {
	if c == nil || c.snapshot == nil {
		return nil
	}
	if err := c.snapshot.Require(); err != nil {
		return nil
	}
	return c.snapshot
}

func (c *Controller) resolveMusicListProfile(override *drawing.DetailedProfileCardRequest, region renderregion.Value) *drawing.DetailedProfileCardRequest {
	if override != nil {
		return new(*override)
	}
	if snap := c.currentSnapshot(); snap != nil {
		if profile := snap.DetailedProfile(region); profile != nil {
			return new(*profile)
		}
	}
	return nil
}

func (c *Controller) profileCard(region renderregion.Value) drawing.ProfileCardRequest {
	if snap := c.currentSnapshot(); snap != nil {
		if profile := snap.ProfileCard(region); profile != nil {
			return *profile
		}
	}
	return convertDetailedProfileToCard(c.buildPlaceholderProfile(region))
}

func (c *Controller) resolveProfileCard(override *drawing.ProfileCardRequest, region renderregion.Value) drawing.ProfileCardRequest {
	if override != nil {
		return *override
	}
	return c.profileCard(region)
}

func (c *Controller) profileCardWithMessage(override *drawing.ProfileCardRequest, region renderregion.Value, message *string) drawing.ProfileCardRequest {
	card := c.resolveProfileCard(override, region)
	if message == nil {
		return card
	}
	card.ErrorMessage = new(*message)
	return card
}

func (c *Controller) buildPlaceholderProfile(region renderregion.Value) drawing.DetailedProfileCardRequest {
	leaderPath := assets.ResolveProfilePlaceholderPath(c.assets)
	return drawing.DetailedProfileCardRequest{
		ID:              "service",
		Region:          strings.ToUpper(renderregion.WithDefault(region).String()),
		Nickname:        "Lunabot",
		Source:          "lunabot-service",
		UpdateTime:      time.Now().UnixMilli(),
		Mode:            new("service"),
		IsHideUID:       true,
		LeaderImagePath: leaderPath,
		HasFrame:        false,
		UserCards:       []any{},
	}
}

func convertDetailedProfileToCard(detail drawing.DetailedProfileCardRequest) drawing.ProfileCardRequest {
	source := detail.Source
	if source == "" {
		source = "lunabot-service"
	}
	update := detail.UpdateTime
	if update == 0 {
		update = time.Now().UnixMilli()
	}
	return drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       common.CloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:       "User Data",
				Source:     &source,
				UpdateTime: &update,
				Mode:       common.CloneStringPtr(detail.Mode),
			},
		},
	}
}

func (c *Controller) buildPlayResultIconMap(_ renderregion.Value) map[string]string {
	return map[string]string{
		"not_clear": assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_not_clear.png"),
		"clear":     assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_clear.png"),
		"fc":        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_fc.png"),
		"ap":        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_ap.png"),
	}
}

func (c *Controller) buildUserResults(diff string) map[int]string {
	snap := c.currentSnapshot()
	if snap == nil {
		return nil
	}
	return snap.MusicResults(diff)
}

func (c *Controller) buildDefaultProgressCounts(source DataSource, builder *Builder, diff string) []drawing.PlayProgressCount {
	return c.buildProgressCountsFromResults(source, builder, diff, nil)
}

func (c *Controller) buildUserProgressCounts(source DataSource, builder *Builder, diff string) []drawing.PlayProgressCount {
	snap := c.currentSnapshot()
	if snap == nil {
		return nil
	}
	return c.buildProgressCountsFromResults(source, builder, diff, snap.MusicResults(diff))
}

func (c *Controller) buildProgressCountsFromResults(source DataSource, builder *Builder, diff string, userResults map[int]string) []drawing.PlayProgressCount {
	countMap := make(map[int]*drawing.PlayProgressCount)
	now := time.Now().UnixMilli()
	for _, musicInfo := range source.GetMusics() {
		if musicInfo == nil || musicInfo.PublishedAt > now {
			continue
		}
		if _, blocked := hiddenMusicIDs[musicInfo.ID]; blocked {
			continue
		}
		level := builder.GetDifficultyLevel(musicInfo.ID, diff)
		if level == 0 {
			continue
		}
		entry := countMap[level]
		if entry == nil {
			entry = &drawing.PlayProgressCount{Level: level}
			countMap[level] = entry
		}
		entry.Total++
		switch strings.ToLower(strings.TrimSpace(userResults[musicInfo.ID])) {
		case "ap":
			entry.Ap++
			entry.Fc++
			entry.Clear++
		case "fc":
			entry.Fc++
			entry.Clear++
		case "clear":
			entry.Clear++
		default:
			entry.NotClear++
		}
	}
	return flattenProgressCounts(countMap)
}

func flattenProgressCounts(countMap map[int]*drawing.PlayProgressCount) []drawing.PlayProgressCount {
	if len(countMap) == 0 {
		return nil
	}

	levels := make([]int, 0, len(countMap))
	for level := range countMap {
		levels = append(levels, level)
	}
	sort.Ints(levels)

	counts := make([]drawing.PlayProgressCount, 0, len(levels))
	for _, level := range levels {
		counts = append(counts, *countMap[level])
	}
	return counts
}

func ensureDetailComboRewards(combo map[string][]drawing.MusicComboReward) map[string][]drawing.MusicComboReward {
	if combo == nil {
		combo = make(map[string][]drawing.MusicComboReward)
	}
	for _, diff := range []string{"hard", "expert", "master", "append"} {
		if _, ok := combo[diff]; !ok {
			combo[diff] = []drawing.MusicComboReward{}
		}
	}
	return combo
}

func (c *Controller) resolveStaticIcon(explicit *string, filename string) *string {
	if explicit != nil {
		value := strings.TrimSpace(*explicit)
		if value != "" {
			return &value
		}
	}

	path := filepath.ToSlash(filepath.Join(assets.StaticImagesDir, filename))
	if c != nil && c.assets != nil {
		if existing := c.assets.FirstExisting(path); existing != "" {
			path = assets.MakeRelative(c.assets.Primary(), existing)
		}
	}
	return &path
}

func (c *Controller) resolveMusicChartMeta(region renderregion.Value, musicID int, difficulty string) map[string]any {
	diff := normalizeDifficulty(difficulty)
	if diff == "" || musicID <= 0 {
		return nil
	}

	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region.String()); len(payload) > 0 {
			if item := findMusicMeta(payload, musicID, diff); item != nil {
				return item
			}
		}
	}

	if snap := c.currentSnapshot(); snap != nil {
		if payload := snap.MusicMetaBytes(); len(payload) > 0 {
			if item := findMusicMeta(payload, musicID, diff); item != nil {
				return item
			}
		}
	}

	return nil
}

func matchesMusicKeyword(source DataSource, musicInfo *masterdata.Music, keyword string) bool {
	if musicInfo == nil {
		return false
	}
	if strings.Contains(strings.ToLower(musicInfo.Title), keyword) {
		return true
	}
	if pronunciation := strings.ToLower(strings.TrimSpace(musicInfo.Pronunciation)); pronunciation != "" && strings.Contains(pronunciation, keyword) {
		return true
	}
	if tags, err := source.GetMusicTags(musicInfo.ID); err == nil {
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag), keyword) {
				return true
			}
		}
	}
	if titles, err := source.GetMusicLocalizedTitles(musicInfo.ID); err == nil {
		for _, title := range titles {
			if strings.Contains(strings.ToLower(strings.TrimSpace(title)), keyword) {
				return true
			}
		}
	}
	return false
}
