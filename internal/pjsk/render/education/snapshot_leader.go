package education

import (
	"bytes"
	"sort"
	"strings"

	"haruki-cloud/utils/drawing"
	"haruki-cloud/utils/logger"
)

var leaderCountDebugLogger = logger.NewLoggerFromGlobal("LeaderCount")

func (c *Controller) BuildLeaderCountRequestFromSnapshot(query LeaderCountQuery) (*drawing.LeaderCountRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	playCountByCharacter := make(map[int]int, 26)
	missionRequirements, maxPlayLimit := ctx.source.GetLeaderMissionRequirements()
	exCountByCharacter := make(map[int]int)
	exLevelByCharacter := make(map[int]int)
	hasPlayLiveMission := false
	playLiveMissionCount := 0
	playLiveExMissionCount := 0
	status101Count := 0

	for _, item := range ctx.raw.UserCharacterMissionV2s {
		if item.CharacterID <= 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.CharacterMissionType)) {
		case "play_live":
			playCountByCharacter[item.CharacterID] = item.Progress
			hasPlayLiveMission = true
			playLiveMissionCount++
		case "play_live_ex":
			exCountByCharacter[item.CharacterID] = item.Progress
			if _, ok := exLevelByCharacter[item.CharacterID]; !ok {
				exLevelByCharacter[item.CharacterID] = 0
			}
			playLiveExMissionCount++
		}
	}

	if !hasPlayLiveMission {
		for _, item := range ctx.raw.UserCharacterLiveUsageCounts {
			if item.CharacterID <= 0 || !strings.EqualFold(item.CharacterLiveUsageType, "leader") {
				continue
			}
			playCountByCharacter[item.CharacterID] = item.UsageCount
		}
	}

	for _, item := range ctx.raw.UserCharacterMissionV2Statuses {
		if item.CharacterID <= 0 || item.ParameterGroupID != 101 {
			continue
		}
		status101Count++
		if item.Seq > exLevelByCharacter[item.CharacterID] {
			exLevelByCharacter[item.CharacterID] = item.Seq
		}
		exCountByCharacter[item.CharacterID] += leaderMissionRequirementForSeq(missionRequirements, item.Seq)
	}

	leaders := make([]drawing.LeaderCountInfo, 0, 26)
	for charID := 1; charID <= 26; charID++ {
		playCount := playCountByCharacter[charID]
		leaders = append(leaders, drawing.LeaderCountInfo{
			CharaID:       charID,
			CharaIconPath: c.characterIconPath(charID),
			PlayCount:     playCount,
			ExLevel:       exLevelByCharacter[charID],
			ExCount:       exCountByCharacter[charID],
		})
	}
	sort.SliceStable(leaders, func(i, j int) bool {
		totalI := leaders[i].PlayCount + leaders[i].ExCount
		totalJ := leaders[j].PlayCount + leaders[j].ExCount
		if totalI == totalJ {
			return leaders[i].CharaID < leaders[j].CharaID
		}
		return totalI > totalJ
	})

	maxPlay := maxPlayLimit
	if maxPlay <= 0 {
		for _, item := range leaders {
			if item.PlayCount > maxPlay {
				maxPlay = item.PlayCount
			}
		}
	}

	rawBytes, rawBytesErr := ctx.snapshot.RawBytes()
	leaderCountDebugLogger.Debugf(
		"leader-count snapshot summary: region=%s user_id=%d mission_v2s=%d play_live=%d play_live_ex=%d mission_v2_statuses=%d status_101=%d live_usage=%d requirements_101=%d max_play_limit=%d has_key_userCharacterMissionV2s=%t has_key_userCharacterMissionV2Statuses=%t has_key_userCharacterMissionStatuses=%t has_key_userCharacterMissionStatus=%t raw_bytes_err=%v",
		ctx.region.String(),
		ctx.raw.UserGamedata.UserID,
		len(ctx.raw.UserCharacterMissionV2s),
		playLiveMissionCount,
		playLiveExMissionCount,
		len(ctx.raw.UserCharacterMissionV2Statuses),
		status101Count,
		len(ctx.raw.UserCharacterLiveUsageCounts),
		len(missionRequirements),
		maxPlayLimit,
		bytes.Contains(rawBytes, []byte(`"userCharacterMissionV2s"`)),
		bytes.Contains(rawBytes, []byte(`"userCharacterMissionV2Statuses"`)),
		bytes.Contains(rawBytes, []byte(`"userCharacterMissionStatuses"`)),
		bytes.Contains(rawBytes, []byte(`"userCharacterMissionStatus"`)),
		rawBytesErr,
	)

	return c.BuildLeaderCountRequest(drawing.LeaderCountRequest{
		Profile:      *ctx.profile,
		LeaderCounts: leaders,
		MaxPlayCount: maxPlay,
	})
}
