package sekai

// RankDataPoint is a single ranking snapshot.
type RankDataPoint struct {
	UserID    string `json:"userId"`
	Score     int    `json:"score"`
	Rank      int    `json:"rank"`
	Timestamp int64  `json:"timestamp"`
}

// WorldBloomRankDataPoint extends RankDataPoint with a character ID for World Bloom events.
type WorldBloomRankDataPoint struct {
	RankDataPoint
	CharacterID *int `json:"characterId,omitempty"`
}

// RankingUserData contains basic user information returned alongside ranking data.
type RankingUserData struct {
	UserID         string `json:"userId"`
	Name           string `json:"name"`
	CheerfulTeamID *int   `json:"cheerfulTeamId,omitempty"`
}

// LatestRankingResponse is returned by GetLatestRankingByRank / GetLatestRankingByUser.
type LatestRankingResponse struct {
	RankData RankDataPoint   `json:"rankData"`
	UserData RankingUserData `json:"userData"`
}

// WorldBloomLatestRankingResponse is returned by GetLatestWorldBloomRankingByRank /
// GetLatestWorldBloomRankingByUser.
type WorldBloomLatestRankingResponse struct {
	RankData WorldBloomRankDataPoint `json:"rankData"`
	UserData RankingUserData         `json:"userData"`
}

// TraceRankingResponse is returned by TraceRankingByRank / TraceRankingByUser.
type TraceRankingResponse struct {
	RankData []RankDataPoint `json:"rankData"`
	UserData RankingUserData `json:"userData"`
}

// WorldBloomTraceRankingResponse is returned by TraceWorldBloomRankingByRank /
// TraceWorldBloomRankingByUser.
type WorldBloomTraceRankingResponse struct {
	RankData []WorldBloomRankDataPoint `json:"rankData"`
	UserData RankingUserData           `json:"userData"`
}

// RankingLine is one border-score entry returned by GetRankingLines /
// GetWorldBloomRankingLines.
type RankingLine struct {
	Rank      int   `json:"rank"`
	Score     int   `json:"score"`
	Timestamp int64 `json:"timestamp"`
}

// ScoreGrowthPoint is one data point returned by GetRankingScoreGrowth /
// GetWorldBloomRankingScoreGrowth.
type ScoreGrowthPoint struct {
	Rank             int    `json:"rank"`
	ScoreLatest      int    `json:"scoreLatest"`
	ScoreEarlier     *int   `json:"scoreEarlier,omitempty"`
	TimestampLatest  int64  `json:"timestampLatest"`
	TimestampEarlier *int64 `json:"timestampEarlier,omitempty"`
	TimeDiff         *int64 `json:"timeDiff,omitempty"`
	Growth           *int   `json:"growth,omitempty"`
}

// UserEventData contains a user's name record for a specific event,
// as returned by GetUserEventData.
type UserEventData struct {
	UserID         string `json:"userId"`
	Name           string `json:"name"`
	CheerfulTeamID *int   `json:"cheerfulTeamId,omitempty"`
}

// EventStatusResponse is returned by GetEventStatus and describes the latest
// heartbeat state of the tracker for a given event.
type EventStatusResponse struct {
	Timestamp  int64  `json:"timestamp"`
	Status     int8   `json:"status"`
	StatusDesc string `json:"statusDesc"`
	TimeAgo    int64  `json:"timeAgo"`
}
