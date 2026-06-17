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

// Legacy raw tracker shapes kept for older SK controller test fixtures.
// Production tracker access uses the Cloud SK semantic response types below.
type LatestRankingResponse struct {
	RankData RankDataPoint   `json:"rankData"`
	UserData RankingUserData `json:"userData"`
}

type WorldBloomLatestRankingResponse struct {
	RankData WorldBloomRankDataPoint `json:"rankData"`
	UserData RankingUserData         `json:"userData"`
}

type TraceRankingResponse struct {
	RankData []RankDataPoint `json:"rankData"`
	UserData RankingUserData `json:"userData"`
}

type WorldBloomTraceRankingResponse struct {
	RankData []WorldBloomRankDataPoint `json:"rankData"`
	UserData RankingUserData           `json:"userData"`
}

type BatchTraceRankingItem struct {
	Rank     int             `json:"rank"`
	RankData []RankDataPoint `json:"rankData"`
}

type BatchTraceRankingResponse struct {
	Items []BatchTraceRankingItem `json:"items"`
}

type BatchWorldBloomTraceRankingItem struct {
	Rank     int                       `json:"rank"`
	RankData []WorldBloomRankDataPoint `json:"rankData"`
}

type BatchWorldBloomTraceRankingResponse struct {
	Items []BatchWorldBloomTraceRankingItem `json:"items"`
}

type ScoreGrowthPoint struct {
	Rank             int    `json:"rank"`
	ScoreLatest      int    `json:"scoreLatest"`
	ScoreEarlier     *int   `json:"scoreEarlier,omitempty"`
	TimestampLatest  int64  `json:"timestampLatest"`
	TimestampEarlier *int64 `json:"timestampEarlier,omitempty"`
	TimeDiff         *int64 `json:"timeDiff,omitempty"`
	Growth           *int   `json:"growth,omitempty"`
}

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

type LeaderboardMeta struct {
	Server      string `json:"server"`
	EventID     int    `json:"eventId"`
	Scope       string `json:"scope"`
	CharacterID *int   `json:"characterId,omitempty"`
	FetchedAt   int64  `json:"fetchedAt"`
}

type SubjectTraceMeta struct {
	SubjectType    string  `json:"subjectType"`
	Subject        string  `json:"subject"`
	ResolvedUserID *string `json:"resolvedUserId,omitempty"`
	ResolvedRank   *int    `json:"resolvedRank,omitempty"`
}

type CloudRankInfo struct {
	Rank            int     `json:"rank"`
	UserID          *string `json:"userId,omitempty"`
	Name            string  `json:"name"`
	Score           int     `json:"score"`
	Timestamp       int64   `json:"timestamp"`
	AverageRound    *int    `json:"averageRound,omitempty"`
	AveragePt       *int    `json:"averagePt,omitempty"`
	LatestPt        *int    `json:"latestPt,omitempty"`
	Speed           *int    `json:"speed,omitempty"`
	Min20Time3Speed *int    `json:"min20Times3Speed,omitempty"`
	HourRound       *int    `json:"hourRound,omitempty"`
	RecordStartAt   *int64  `json:"recordStartAt,omitempty"`
	SpeedWindow     *int64  `json:"speedWindow,omitempty"`
	CharacterID     *int    `json:"characterId,omitempty"`
}

type CloudRankQueryResponse struct {
	Meta     LeaderboardMeta `json:"meta"`
	Ranks    []CloudRankInfo `json:"ranks,omitempty"`
	Previous *CloudRankInfo  `json:"previous,omitempty"`
	Next     *CloudRankInfo  `json:"next,omitempty"`
}

type CloudCheckRoomResponse struct {
	Meta     LeaderboardMeta `json:"meta"`
	Rank     CloudRankInfo   `json:"rank"`
	Ranks    []CloudRankInfo `json:"ranks,omitempty"`
	Previous *CloudRankInfo  `json:"previous,omitempty"`
	Next     *CloudRankInfo  `json:"next,omitempty"`
}

type CloudLineResponse struct {
	Meta  LeaderboardMeta `json:"meta"`
	Ranks []CloudRankInfo `json:"ranks,omitempty"`
}

type CloudSpeedResponse struct {
	Meta            LeaderboardMeta `json:"meta"`
	Speeds          []CloudRankInfo `json:"speeds,omitempty"`
	IntervalSeconds int64           `json:"intervalSeconds"`
	UnitSeconds     int64           `json:"unitSeconds"`
}

type CloudTraceResponse struct {
	Meta     LeaderboardMeta  `json:"meta"`
	Subject  SubjectTraceMeta `json:"subject"`
	RankData []CloudRankInfo  `json:"rankData,omitempty"`
}
