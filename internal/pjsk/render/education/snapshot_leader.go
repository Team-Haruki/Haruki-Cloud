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

	missionRequirements, maxPlayLimit := ctx.source.GetLeaderMissionRequirements()
	progress := collectLeaderMissionProgress(ctx.raw)
	missionStatuses := leaderMissionStatuses(ctx.snapshot, ctx.raw)
	status101Count := applyLeaderMissionStatuses(progress, missionStatuses, missionRequirements)
	leaders := c.buildLeaderCounts(progress)
	maxPlay := leaderMaximumPlayCount(leaders, maxPlayLimit)

	rawBytes, rawBytesErr := ctx.snapshot.RawBytes()
	leaderCountDebugLogger.DebugContext(c.traceContext(), "leader count snapshot summarized",
		"region", ctx.region.String(),
		"mission_v2_count", len(ctx.raw.UserCharacterMissionV2s),
		"play_live_count", progress.playLiveMissionCount,
		"play_live_ex_count", progress.playLiveExMissionCount,
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

type leaderMissionProgress struct {
	playCounts             map[int]int
	exCounts               map[int]int
	exLevels               map[int]int
	hasPlayLiveEx          map[int]bool
	hasPlayLiveMission     bool
	playLiveMissionCount   int
	playLiveExMissionCount int
}

func collectLeaderMissionProgress(raw *rendersnapshot.RawUserData) *leaderMissionProgress {
	progress := &leaderMissionProgress{
		playCounts:    make(map[int]int, 26),
		exCounts:      make(map[int]int),
		exLevels:      make(map[int]int),
		hasPlayLiveEx: make(map[int]bool),
	}
	for _, item := range raw.UserCharacterMissionV2s {
		if item.CharacterID <= 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.CharacterMissionType)) {
		case "play_live":
			progress.playCounts[item.CharacterID] = item.Progress
			progress.hasPlayLiveMission = true
			progress.playLiveMissionCount++
		case "play_live_ex":
			progress.exCounts[item.CharacterID] = item.Progress
			progress.hasPlayLiveEx[item.CharacterID] = true
			progress.playLiveExMissionCount++
		}
	}
	if !progress.hasPlayLiveMission {
		collectLeaderUsageCounts(progress.playCounts, raw.UserCharacterLiveUsageCounts)
	}
	return progress
}

func collectLeaderUsageCounts(playCounts map[int]int, usageCounts []rendersnapshot.RawUserCharacterLiveUsageCount) {
	for _, item := range usageCounts {
		if item.CharacterID <= 0 || !strings.EqualFold(item.CharacterLiveUsageType, "leader") {
			continue
		}
		playCounts[item.CharacterID] = item.UsageCount
	}
}

func applyLeaderMissionStatuses(progress *leaderMissionProgress, statuses []rendersnapshot.RawUserCharacterMissionV2Status, requirements []LeaderMissionRequirement) int {
	statusCount := 0
	for _, item := range statuses {
		if item.CharacterID <= 0 || item.ParameterGroupID != 101 {
			continue
		}
		statusCount++
		progress.exLevels[item.CharacterID] = max(progress.exLevels[item.CharacterID], item.Seq)
		progress.exCounts[item.CharacterID] += leaderMissionRequirementForSeq(requirements, item.Seq)
	}
	for charID := 1; charID <= 26; charID++ {
		if progress.hasPlayLiveEx[charID] {
			progress.exLevels[charID]++
		}
	}
	return statusCount
}

func (c *Controller) buildLeaderCounts(progress *leaderMissionProgress) []drawing.LeaderCountInfo {
	leaders := make([]drawing.LeaderCountInfo, 0, 26)
	for charID := 1; charID <= 26; charID++ {
		leaders = append(leaders, drawing.LeaderCountInfo{
			CharaID:       charID,
			CharaIconPath: c.characterIconPath(charID),
			PlayCount:     progress.playCounts[charID],
			ExLevel:       progress.exLevels[charID],
			ExCount:       progress.exCounts[charID],
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
	return leaders
}

func leaderMaximumPlayCount(leaders []drawing.LeaderCountInfo, configuredMaximum int) int {
	if configuredMaximum > 0 {
		return configuredMaximum
	}
	maximum := 0
	for _, item := range leaders {
		maximum = max(maximum, item.PlayCount)
	}
	return maximum
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
