package sk

import (
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

const trackerRealtimeTailMaxLagSeconds = int64(30 * 24 * time.Hour / time.Second)

func (c *Controller) enrichRankInfoByRank(server string, eventID, rank int, wlCharacterID *int, info *drawing.RankInfo) {
}

func (c *Controller) enrichRankInfoByUser(server string, eventID int, userID int64, wlCharacterID *int, info *drawing.RankInfo) {
}

func applyRankInfoMetrics(info *drawing.RankInfo, samples []trackerScoreSample) {
	applyRankInfoMetricsAt(info, samples, time.Now().UTC())
}

func applyRankInfoMetricsAt(info *drawing.RankInfo, samples []trackerScoreSample, now time.Time) {
	if info == nil || len(samples) == 0 {
		return
	}

	normalized := make([]trackerScoreSample, 0, len(samples))
	for _, sample := range samples {
		if sample.timestamp <= 0 {
			continue
		}
		normalized = append(normalized, sample)
	}
	if len(normalized) == 0 {
		return
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalizeTrackerUnixSeconds(normalized[i].timestamp) < normalizeTrackerUnixSeconds(normalized[j].timestamp)
	})

	info.RecordStartAt = drawing.Int64Ptr(formatTrackerTimestamp(normalized[0].timestamp))
	if len(normalized) < 2 {
		return
	}

	deltas := make([]int, 0, len(normalized)-1)
	for i := 1; i < len(normalized); i++ {
		diff := normalized[i].score - normalized[i-1].score
		if diff > 0 {
			deltas = append(deltas, diff)
		}
	}
	if len(deltas) > 0 {
		latest := deltas[len(deltas)-1]
		info.LatestPt = drawing.IntPtr(latest)

		avgWindow := deltas
		if len(avgWindow) > 10 {
			avgWindow = avgWindow[len(avgWindow)-10:]
		}
		sum := 0
		for _, value := range avgWindow {
			sum += value
		}
		roundCount := len(avgWindow)
		if roundCount > 0 {
			avg := sum / roundCount
			info.AverageRound = drawing.IntPtr(roundCount)
			info.AveragePt = drawing.IntPtr(avg)
		}
	}

	last := normalized[len(normalized)-1]
	endSec := effectiveTrackerWindowEndUnixSeconds(last.timestamp, now)

	// Speed/HourRound use the last ~1h window to match tracker "近1小时" semantics.
	hourStart := endSec - 60*60
	hourBaseIdx := findWindowBaselineIndex(normalized, hourStart)
	if hourBaseIdx >= 0 {
		hourBase := normalized[hourBaseIdx]
		hourBaseSec := normalizeTrackerUnixSeconds(hourBase.timestamp)
		if endSec > hourBaseSec {
			hourGain := last.score - hourBase.score
			if hourGain < 0 {
				hourGain = 0
			}
			hourElapsed := endSec - hourBaseSec
			speed := int((int64(hourGain) * 3600) / hourElapsed)
			info.Speed = drawing.IntPtr(speed)
		}

		hourRound := countPositiveDeltas(normalized[hourBaseIdx:])
		info.HourRound = drawing.IntPtr(hourRound)
	}

	// 20min×3 is computed as gain in last 20min multiplied by 3.
	windowStart := endSec - 20*60
	windowBaseIdx := findWindowBaselineIndex(normalized, windowStart)
	if windowBaseIdx >= 0 {
		windowBase := normalized[windowBaseIdx]
		windowGain := last.score - windowBase.score
		if windowGain < 0 {
			windowGain = 0
		}
		windowSpeed := windowGain * 3
		info.Min20Time3Speed = drawing.IntPtr(windowSpeed)
	}
}

func normalizeTrackerUnixSeconds(ts int64) int64 {
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func effectiveTrackerWindowEndUnixSeconds(lastTimestamp int64, now time.Time) int64 {
	lastSec := normalizeTrackerUnixSeconds(lastTimestamp)
	if lastSec <= 0 {
		return lastSec
	}

	nowSec := now.UTC().Unix()
	if nowSec <= lastSec {
		return lastSec
	}
	if nowSec-lastSec > trackerRealtimeTailMaxLagSeconds {
		return lastSec
	}
	return nowSec
}

func appendIdleTrackerRankTraceAt(ranks []drawing.RankInfo, now time.Time) []drawing.RankInfo {
	if len(ranks) == 0 {
		return ranks
	}

	last := ranks[len(ranks)-1]
	lastSec := normalizeTrackerUnixSeconds(last.Time)
	endSec := effectiveTrackerWindowEndUnixSeconds(last.Time, now)
	if endSec <= lastSec {
		return ranks
	}

	idle := last
	idle.Time = time.Unix(endSec, 0).UTC().UnixMilli()
	return append(ranks, idle)
}

func findWindowBaselineIndex(samples []trackerScoreSample, windowStart int64) int {
	if len(samples) == 0 {
		return -1
	}
	baseline := -1
	for i := range samples {
		sec := normalizeTrackerUnixSeconds(samples[i].timestamp)
		if sec <= windowStart {
			baseline = i
			continue
		}
		break
	}
	if baseline >= 0 {
		return baseline
	}
	return 0
}

func countPositiveDeltas(samples []trackerScoreSample) int {
	if len(samples) < 2 {
		return 0
	}
	count := 0
	for i := 1; i < len(samples); i++ {
		if samples[i].score-samples[i-1].score > 0 {
			count++
		}
	}
	return count
}
