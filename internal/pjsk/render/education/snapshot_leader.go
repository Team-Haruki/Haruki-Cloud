package education

import (
	"bytes"
	"sort"
	"strings"

	json "haruki-cloud/internal/jsonutil"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/utils/logger"
)

var leaderCountDebugLogger = logger.NewLoggerFromGlobal("LeaderCount")

func (c *Controller) BuildLeaderCountRequestFromSnapshot(query LeaderCountQuery) (*drawing.LeaderCountRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	playCountByCharacter := make(map[int]int, 26)
	missionRequirements, maxPlayLimit := ctx.source.GetLeaderMissionRequirements()
	exCountByCharacter := make(map[int]int)
	exLevelByCharacter := make(map[int]int)
	exProgressByCharacter := make(map[int]int)
	hasPlayLiveExByCharacter := make(map[int]bool)
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
			exProgressByCharacter[item.CharacterID] = item.Progress
			hasPlayLiveExByCharacter[item.CharacterID] = true
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

	missionStatuses := leaderMissionStatuses(ctx.snapshot, ctx.raw)
	for _, item := range missionStatuses {
		if item.CharacterID <= 0 || item.ParameterGroupID != 101 {
			continue
		}
		status101Count++
		if item.Seq > exLevelByCharacter[item.CharacterID] {
			exLevelByCharacter[item.CharacterID] = item.Seq
		}
		exCountByCharacter[item.CharacterID] += leaderMissionRequirementForSeq(missionRequirements, item.Seq)
	}

	for charID := 1; charID <= 26; charID++ {
		if !hasPlayLiveExByCharacter[charID] {
			continue
		}
		exLevelByCharacter[charID]++
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
	leaderCountDebugLogger.DebugContext(c.traceContext(), "leader count snapshot summarized",
		"region", ctx.region.String(),
		"mission_v2_count", len(ctx.raw.UserCharacterMissionV2s),
		"play_live_count", playLiveMissionCount,
		"play_live_ex_count", playLiveExMissionCount,
		"mission_v2_status_count", len(missionStatuses),
		"status_101_count", status101Count,
		"live_usage_count", len(ctx.raw.UserCharacterLiveUsageCounts),
		"requirements_101_count", len(missionRequirements),
		"max_play_limit", maxPlayLimit,
		"has_mission_v2", bytes.Contains(rawBytes, []byte(`"userCharacterMissionV2s"`)),
		"has_mission_v2_statuses", bytes.Contains(rawBytes, []byte(`"userCharacterMissionV2Statuses"`)),
		"has_compact_mission_v2_statuses", bytes.Contains(rawBytes, []byte(`"compactUserCharacterMissionV2Statuses"`)),
		"has_mission_statuses", bytes.Contains(rawBytes, []byte(`"userCharacterMissionStatuses"`)),
		"has_mission_status", bytes.Contains(rawBytes, []byte(`"userCharacterMissionStatus"`)),
		"raw_bytes_ok", rawBytesErr == nil,
	)

	return c.BuildLeaderCountRequest(drawing.LeaderCountRequest{
		Profile:      *ctx.profile,
		LeaderCounts: leaders,
		MaxPlayCount: maxPlay,
	})
}

func leaderMissionStatuses(snapshot rendersnapshot.Snapshot, raw *rendersnapshot.RawUserData) []rendersnapshot.RawUserCharacterMissionV2Status {
	if statuses := rendersnapshot.ResolveCharacterMissionV2Statuses(raw); len(statuses) > 0 {
		return statuses
	}
	if snapshot == nil {
		return nil
	}
	rawBytes, err := snapshot.RawBytes()
	if err != nil || len(rawBytes) == 0 {
		return nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		return nil
	}

	for _, key := range []string{"userCharacterMissionStatuses", "userCharacterMissionStatus"} {
		if items := decodeLegacyLeaderMissionStatuses(payload[key]); len(items) > 0 {
			return items
		}
	}
	if items := rendersnapshot.DecodeCompactCharacterMissionV2Statuses(payload["compactUserCharacterMissionV2Statuses"]); len(items) > 0 {
		return items
	}
	return nil
}

func decodeLegacyLeaderMissionStatuses(raw json.RawMessage) []rendersnapshot.RawUserCharacterMissionV2Status {
	if len(raw) == 0 {
		return nil
	}

	var items []rendersnapshot.RawUserCharacterMissionV2Status
	if err := json.Unmarshal(raw, &items); err == nil && len(items) > 0 {
		return items
	}

	var item rendersnapshot.RawUserCharacterMissionV2Status
	if err := json.Unmarshal(raw, &item); err == nil && item.CharacterID > 0 {
		return []rendersnapshot.RawUserCharacterMissionV2Status{item}
	}

	return nil
}
